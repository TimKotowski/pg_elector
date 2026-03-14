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

	contextWatcher *ContextWatcher
}

type LeaderCallback struct {
	OnStartedLeading func()
	OnStoppedLeading func()
	OnNewLeader      func(nodeId string)
}

func NewLeaderElector(ctx context.Context, d driver.Driver, config *Config) (*Elector, error) {
	nodeId, err := getNodeId()
	if err != nil {
		return nil, err
	}
	handler := func() {
		err := d.GetQuerier().ReleaseLeadership(context.Background(), driver.BasePrams{
			Name:     config.Name,
			LeaderId: nodeId,
		})
		if err != nil {
			log.Println("Unable to release leadership gracefully", err)
		}
	}

	elector := &Elector{
		ctx:    ctx,
		nodeId: nodeId,
		leaderCallback: LeaderCallback{
			OnStoppedLeading: func() { return },
			OnStartedLeading: func() { return },
			OnNewLeader:      func(nodeId string) { return },
		},
		driver: d,
		config: config,
		state:  FOLLOWER,
		mutex:  sync.Mutex{},
	}

	if config.ReleaseOnCancel {
		elector.contextWatcher = NewContextWatcher(handler, ctx)
	}

	return elector, nil
}

func (e *Elector) Start(ctx context.Context) error {
	if e.config.ReleaseOnCancel {
		e.contextWatcher.Watch()
	}

	electionTimer := time.NewTimer(0)
	for {
		leader, err := e.attemptToAcquireLeadership()
		if err != nil {
			// TODO: Swallow error for now. Improve later.
			continue
		}

		if leader {
			e.changeState(LEADER)
			e.runBlockingLeadershipLoop(ctx)
		}

		jitter := JitterDuration(e.config.ElectionClock.ElectionJitterInterval)
		electionTimer.Reset(e.config.ElectionClock.ElectionInterval + jitter)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-electionTimer.C:
		}
	}
}

func (e *Elector) isLeader() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return e.state == LEADER
}

func (e *Elector) isFollower() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return e.state == FOLLOWER
}

func (e *Elector) runBlockingLeadershipLoop(ctx context.Context) {
	renewalTimer := time.NewTicker(e.config.ElectionClock.LeadRetryPeriod)
	deadlineTimer := time.NewTimer(e.config.ElectionClock.LeaderDeadline)
	handOffLeadershipLoss := func() {
		e.changeState(FOLLOWER)
		renewalTimer.Stop()
		deadlineTimer.Stop()
	}

	defer handOffLeadershipLoss()
	for {
		select {
		case <-renewalTimer.C:
			ctxTimeout, cancel := context.WithTimeout(ctx, e.config.ElectionClock.LeaderDeadline)
			renewal, err := e.renewLeadership(ctxTimeout)
			cancel()
			if renewal == NO_ROW_AFFECTED {
				return
			}
			if err != nil {
				continue
			}

			if !deadlineTimer.Stop() {
				select {
				case <-deadlineTimer.C:
					return
				default:
				}
			}
			deadlineTimer.Reset(e.config.ElectionClock.LeaderDeadline)

		case <-deadlineTimer.C:
			return

		case <-ctx.Done():
			if e.config.ReleaseOnCancel {
				<-e.contextWatcher.Release()
			}
			return
		}
	}
}

func (e *Elector) attemptToAcquireLeadership() (bool, error) {
	return e.driver.GetQuerier().AcquireLeadership(context.Background(), driver.AcquireLeadershipParams{
		BasePrams: driver.BasePrams{
			Name:     e.config.Name,
			LeaderId: e.nodeId,
		},
		LeaseDuration: e.leaseDuration().Seconds(),
	})
}

func (e *Elector) renewLeadership(ctx context.Context) (int64, error) {
	return e.driver.GetQuerier().LeaderRenewal(ctx, driver.LeaderRenewalParams{
		BasePrams: driver.BasePrams{
			Name:     e.config.Name,
			LeaderId: e.nodeId,
		},
		LeseDuration: e.leaseDuration().Seconds(),
	})
}

func (e *Elector) changeState(state State) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.state = state
}

func (e *Elector) leaseDuration() time.Duration {
	electionIntervalMs := e.config.ElectionClock.ElectionInterval
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
