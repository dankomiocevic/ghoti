package cluster

import (
	"testing"

	"github.com/hashicorp/raft"
)

func TestEmptyCluster(t *testing.T) {
	c := NewEmptyCluster()

	err := c.Start()
	if err != nil {
		t.Fatalf("Method Start should always return nil")
	}

	err = c.Join("", "")
	if err != nil {
		t.Fatalf("Method Join should always return nil")
	}

	err = c.Remove("")
	if err != nil {
		t.Fatalf("Method Remove should always return nil")
	}

	err = c.Bootstrap().Error()
	if err != nil {
		t.Fatalf("Method bootstrap should always return nil promise")
	}

	leader := c.IsLeader()
	if !leader {
		t.Fatalf("Empty cluster must always have one leader")
	}

	node := c.GetLeader()
	if len(node) > 0 {
		t.Fatalf("Empty cluster must have empty leader node name")
	}

	err = c.Shutdown().Error()
	if err != nil {
		t.Fatalf("Method shudtdown should always return nil promise")
	}

	state := c.state()
	if state != raft.Leader {
		t.Fatalf("Empty cluster state must always be leader")
	}
}
