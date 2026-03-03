package cluster

import (
	"testing"
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

	leader := c.IsLeader()
	if !leader {
		t.Fatalf("Empty cluster must always have one leader")
	}

	node := c.GetLeader()
	if len(node) > 0 {
		t.Fatalf("Empty cluster must have empty leader node name")
	}

	err = c.Shutdown()
	if err != nil {
		t.Fatalf("Method shutdown should always return nil")
	}
}
