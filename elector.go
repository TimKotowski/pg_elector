package pg_elector

import (
	"context"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TimKotowski/pg_elector/driver"
)

type State string

const (
	NO_ROW_AFFECTED = 0
)

var (
	LEADER   State = "leader"
	FOLLOWER State = "follower"
)

type Elector struct {
	ctx context.Context

	nodeId string

	leaderCallback LeaderCallback

	state State

	driver driver.Driver

	config *Config

	mutex sync.Mutex
}

type LeaderCallback struct {
	OnStartedLeading func()
	OnStoppedLeading func()
	OnNewLeader      func(nodeId string)
}

func NewLeaderElector(ctx context.Context, driver driver.Driver, config *Config) (*Elector, error) {
	nodeId, err := getNodeId()
	if err != nil {
		return nil, err
	}

	return &Elector{
		ctx:    ctx,
		nodeId: nodeId,
		leaderCallback: LeaderCallback{
			OnStoppedLeading: func() { return },
			OnStartedLeading: func() { return },
			OnNewLeader:      func(nodeId string) { return },
		},
		driver: driver,
		config: config,
		state:  FOLLOWER,
		mutex:  sync.Mutex{},
	}, nil
}

func (e *Elector) Start(ctx context.Context) error {
	electionTimer := time.NewTimer(0)

	for {
		leader, err := e.attemptToAcquireLeadership()
		if err != nil {
			return err
		}

		if leader {
			e.runBlockingLeadershipLoop(ctx)
		}

		if e.config.ReleaseOnCancel {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		jitter := JitterDuration(e.config.ElectionClock.ElectionJitterInterval)
		electionTimer.Reset(e.config.ElectionClock.ElectionInterval + jitter)
		select {
		case <-electionTimer.C:
		}
	}
}

func (e *Elector) attemptToAcquireLeadership() (bool, error) {
	return e.driver.GetQuerier().AcquireLeadership(context.Background(), driver.AcquireLeadershipParams{
		BasePrams: driver.BasePrams{
			Name:     e.config.Name,
			LeaderId: e.nodeId,
		},
		LeseDuration: e.leaseDuration().Seconds(),
	})
}

func (e *Elector) runBlockingLeadershipLoop(ctx context.Context) {
	log.Printf("start leadership loop for %v", e.nodeId)

	renewalTimer := time.NewTicker(e.config.ElectionClock.LeadRetryPeriod)
	deadlineTimer := time.NewTimer(e.config.ElectionClock.LeaderDeadline)
	stop := func() {
		renewalTimer.Stop()
		deadlineTimer.Stop()
	}

	for {
		if e.config.ReleaseOnCancel {
			select {
			case <-ctx.Done():
				stop()
				return
			default:
			}
		}

		select {
		case <-renewalTimer.C:
			renewal, err := e.driver.GetQuerier().LeaderRenewal(context.Background(), driver.LeaderRenewalParams{LeaderId: e.nodeId})
			if err != nil || renewal == NO_ROW_AFFECTED {
				stop()
				return
			}

			if !deadlineTimer.Stop() {
				select {
				case <-deadlineTimer.C:
					stop()
					return
				default:
				}
			}

			deadlineTimer.Reset(e.config.ElectionClock.LeaderDeadline)
			log.Printf("nodeId %v renewed", e.nodeId)
		}
	}
}

func (e *Elector) isLeader() bool {
	return e.state == LEADER
}

func (e *Elector) isFollower() bool {
	return e.state == FOLLOWER
}

func (e *Elector) changeState(state State) {
	e.mutex.Lock()
	e.state = state
	e.mutex.Unlock()
}

func (e *Elector) leaseDuration() time.Duration {
	electionIntervalMs := e.config.ElectionClock.ElectionInterval.Milliseconds()
	padding := time.Duration(float64(electionIntervalMs) * 0.5)

	if padding < time.Second*10 {
		padding = time.Second * 10
	}

	if padding > time.Minute*2 {
		// Set a lower ratio if the padding is over 2 minutes.
		padding = time.Duration(float64(electionIntervalMs) * 0.2)
	}

	return e.config.ElectionClock.ElectionInterval + padding
}

func JitterDuration(d time.Duration) time.Duration {
	// [0.5-1.1]
	jitter := 0.5 + rand.Float64()*0.6
	return time.Duration(float64(d) * jitter)
}

func getNodeId() (string, error) {
	// Don't allow super long host names, narrow it down.
	maxHostLength := 80
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	if host == "" {
		host = "default_host"
	}

	if len(host) > maxHostLength {
		host = host[0:maxHostLength]
	}

	nodeId := strings.NewReplacer(".", "_", "-", "_").Replace(host)

	return nodeId + "_" + strings.ReplaceAll(time.Now().UTC().Format("2006_01_02T15_04_05Z07.00000"), ".", "_"), nil
}
