package structs

import "time"

type (
	Connect struct {
		Ip      string
		Port    int
		Timeout time.Duration
		Uid     string
	}
	CloseConnect struct {
		Ip  string
		Uid string
	}
	ControlStruct struct {
		Err     error
		Code    int
		Module  string
		Message string
	}
)
