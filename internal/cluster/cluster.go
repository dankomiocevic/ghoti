package cluster

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Cluster interface {
	Start() error
	Join(string, string) error
	Remove(string) error
	IsLeader() bool
	GetLeader() string
	Shutdown() error
}

type BullyCluster struct {
	config  ClusterConfig
	nodeID  string
	peers   map[string]string // nodeID -> managerAddr
	leader  string
	isUp    bool
	mu      sync.RWMutex
	manager MembershipManager
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func NewCluster(config ClusterConfig) (*BullyCluster, error) {
	c := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	manager, err := GetManager(&config, c)
	if err != nil {
		return nil, err
	}

	c.manager = manager
	return c, nil
}

func (c *BullyCluster) Start() error {
	if len(c.config.ManagerAddr) == 0 {
		return fmt.Errorf("manager address is required")
	}

	err := c.manager.Start()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.isUp = true
	c.mu.Unlock()

	c.wg.Add(1)
	go c.heartbeatLoop()

	return nil
}

func (c *BullyCluster) Shutdown() error {
	c.mu.Lock()
	c.isUp = false
	c.mu.Unlock()

	close(c.stopCh)
	c.manager.Close()
	c.wg.Wait()
	return nil
}

func (c *BullyCluster) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leader == c.nodeID
}

func (c *BullyCluster) GetLeader() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leader
}

func (c *BullyCluster) Join(nodeID, addr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Info("Adding peer to cluster",
		slog.String("node_id", nodeID),
		slog.String("addr", addr),
	)

	c.peers[nodeID] = addr
	return nil
}

func (c *BullyCluster) Remove(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Info("Removing peer from cluster",
		slog.String("node_id", nodeID),
	)

	delete(c.peers, nodeID)
	return nil
}

func (c *BullyCluster) SetLeader(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leader = nodeID
	slog.Info("Leader set",
		slog.String("leader", nodeID),
		slog.String("node_id", c.nodeID),
	)
}

func (c *BullyCluster) Bootstrap() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leader = c.nodeID
	slog.Info("Node bootstrapped as leader",
		slog.String("node_id", c.nodeID),
	)
}

func (c *BullyCluster) GetNodeID() string {
	return c.nodeID
}

func (c *BullyCluster) GetPeers() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string)
	for id, addr := range c.peers {
		result[id] = addr
	}
	return result
}

func (c *BullyCluster) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.IsLeader() {
				c.checkLeader()
			}
		}
	}
}

func (c *BullyCluster) checkLeader() {
	c.mu.RLock()
	leaderID := c.leader
	leaderAddr, ok := c.peers[leaderID]
	c.mu.RUnlock()

	if !ok || leaderID == "" {
		return
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/heartbeat", leaderAddr))
	if err != nil {
		slog.Info("Leader heartbeat failed, starting election",
			slog.String("leader", leaderID),
			slog.Any("error", err),
		)
		c.StartElection()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Info("Leader heartbeat returned non-OK, starting election",
			slog.String("leader", leaderID),
			slog.Int("status", resp.StatusCode),
		)
		c.StartElection()
	}
}

func (c *BullyCluster) StartElection() {
	slog.Info("Starting bully election",
		slog.String("node_id", c.nodeID),
	)

	c.mu.RLock()
	higherPeers := make(map[string]string)
	for id, addr := range c.peers {
		if id > c.nodeID {
			higherPeers[id] = addr
		}
	}
	c.mu.RUnlock()

	if len(higherPeers) == 0 {
		c.DeclareLeader()
		return
	}

	gotResponse := false
	client := &http.Client{Timeout: 3 * time.Second}
	for id, addr := range higherPeers {
		req, err := http.NewRequest("POST",
			fmt.Sprintf("http://%s/election", addr),
			strings.NewReader(fmt.Sprintf(`{"id":"%s"}`, c.nodeID)),
		)
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(c.config.User, c.config.Pass)

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			slog.Info("Higher node responded to election",
				slog.String("higher_node", id),
			)
			gotResponse = true
			resp.Body.Close()
		}
	}

	if !gotResponse {
		c.DeclareLeader()
	}
}

func (c *BullyCluster) DeclareLeader() {
	c.SetLeader(c.nodeID)
	slog.Info("Declaring self as leader",
		slog.String("node_id", c.nodeID),
	)

	c.mu.RLock()
	peers := make(map[string]string)
	for id, addr := range c.peers {
		peers[id] = addr
	}
	c.mu.RUnlock()

	client := &http.Client{Timeout: 2 * time.Second}
	for id, addr := range peers {
		if id == c.nodeID {
			continue
		}
		req, err := http.NewRequest("POST",
			fmt.Sprintf("http://%s/coordinator", addr),
			strings.NewReader(fmt.Sprintf(`{"id":"%s"}`, c.nodeID)),
		)
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(c.config.User, c.config.Pass)
		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("Failed to notify peer of new leader",
				slog.String("peer", id),
				slog.Any("error", err),
			)
		} else {
			resp.Body.Close()
		}
	}
}
