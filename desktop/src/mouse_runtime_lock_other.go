//go:build !windows

package main

func withPortableMutationLock(action func() error) error { return action() }
