package serverstatus

import (
	"fmt"
	"time"
)

type Status struct {
	Version, Description string
	Icon                 []byte
	Members              []Member
	Protocol             int
	Online, Maximum      int
	Latency              time.Duration
}

type Member struct {
	Name, UUID string
}

func (s Status) Compatible(protocol int) bool {
	return protocol < 0 || s.Protocol < 0 || protocol == s.Protocol
}

func (s Status) Players() string { return fmt.Sprintf("%d / %d", s.Online, s.Maximum) }
