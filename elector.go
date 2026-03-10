package pg_elector

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

type State string

var (
	LEADER   State = "leader"
	FOLLOWER State = "follower"
)

const (
	leaseDurationPadding = time.Second * 10
)

type Elector struct {
	ctx context.Context

	nodeId string

	leaderCallback LeaderCallback

	state State

	client *Client

	config *Config

	mutex sync.Mutex
}

type LeaderCallback struct {
	OnStartedLeading func()
	OnStoppedLeading func()
	OnNewLeader      func(nodeId string)
}

func NewLeaderElector(ctx context.Context, client *Client, config *Config) (*Elector, error) {
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
		client: client,
		config: config,
		state:  FOLLOWER,
		mutex:  sync.Mutex{},
	}, nil
}

func (e *Elector) Start() error {
	return nil
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
	return e.config.ElectionClock.ElectionInterval + leaseDurationPadding
}

func getNodeId() (string, error) {
	// Don't allow super long host names, narrow it down.
	maxHostLength := 80
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}

	if len(host) > maxHostLength {
		host = host[0:maxHostLength]
	}

	// Allow double clicks, for logging and debugging to be easier.
	nodeId := strings.ReplaceAll(host, ".", "_")

	return nodeId, nil
}
