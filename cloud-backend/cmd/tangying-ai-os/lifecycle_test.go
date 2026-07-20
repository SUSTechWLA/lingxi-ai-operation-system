package main

import (
	"context"
	"reflect"
	"testing"
)

func TestShutdownDrainsHTTPBeforeStoppingBackgroundWorkers(t *testing.T) {
	order := []string{}
	server := &recordingShutdownServer{order: &order}
	err := shutdownHTTPAndWorkers(context.Background(), server, func() {
		order = append(order, "cancel-workers")
	}, func() {
		order = append(order, "join-workers")
	})
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	want := []string{"drain-http", "cancel-workers", "join-workers"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("shutdown order=%v, want %v", order, want)
	}
}

type recordingShutdownServer struct{ order *[]string }

func (s *recordingShutdownServer) Shutdown(context.Context) error {
	*s.order = append(*s.order, "drain-http")
	return nil
}
