package config

import "time"

type PlayRecord struct {
	Instance string    `json:"instance"`
	Started  time.Time `json:"started"`
	Seconds  int64     `json:"seconds"`
	Server   string    `json:"server,omitempty"`
}
