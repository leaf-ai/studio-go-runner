package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage
import (
	"time"
)

type Storage interface {
	Fetch(name string, unpack bool, output string, timeout time.Duration) (err error)
	Deposit(src string, dest string, timeout time.Duration) (err error)

	Close()
}
