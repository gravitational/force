package main

import (
	"log"

	"testing"

	"github.com/gravitational/trace"
)

func TestFail(t *testing.T) {
	log.Printf("Some logs.")
	err := trace.BadParameter("bla")
	t.Errorf("This test has failed: %v", trace.DebugReport(err))
}

func TestOK(t *testing.T) {
	log.Printf("This test has succeeded.")
}
