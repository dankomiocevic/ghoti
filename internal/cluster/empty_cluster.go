package cluster

type EmptyCluster struct {
}

func NewEmptyCluster() Cluster {
	return &EmptyCluster{}
}

func (c *EmptyCluster) Start() error {
	return nil
}

func (c *EmptyCluster) Join(a, b string) error {
	return nil
}

func (c *EmptyCluster) Remove(a string) error {
	return nil
}

func (c *EmptyCluster) IsLeader() bool {
	return true
}

func (c *EmptyCluster) GetLeader() string {
	return ""
}

func (c *EmptyCluster) Shutdown() error {
	return nil
}
