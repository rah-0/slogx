package main

import "errors"

// ErrConnectionRefused is the simulated database failure.
var ErrConnectionRefused = errors.New("connection refused")
