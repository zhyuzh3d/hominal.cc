package runtime

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMentorHandlerAcceptsBoundedGenerationDeadlineInput(t *testing.T) {
	commands := make(chan RuntimeCommand, 1)
	handler := &mentorHandler{commands: commands}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/lab/deadline",
		bytes.NewBufferString(`{"planned_end":"2026-08-30T01:30:00Z"}`),
	)
	recorder := httptest.NewRecorder()

	go func() {
		command := <-commands
		if command.Kind != "generation_extend" || command.Deadline.PlannedEnd != "2026-08-30T01:30:00Z" {
			command.Reply <- CommandReply{Status: http.StatusBadRequest, Body: map[string]string{"error": "wrong command"}}
			return
		}
		command.Reply <- CommandReply{Status: http.StatusOK, Body: map[string]string{"status": "extended"}}
	}()

	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("deadline endpoint rejected valid input: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMentorHandlerWaitsThroughABusyCognitionTurn(t *testing.T) {
	commands := make(chan RuntimeCommand, 1)
	handler := &mentorHandler{commands: commands}
	request := httptest.NewRequest(http.MethodGet, "/v1/mentor/outbox", nil)
	recorder := httptest.NewRecorder()

	go func() {
		command := <-commands
		time.Sleep(3200 * time.Millisecond)
		command.Reply <- CommandReply{Status: http.StatusOK, Body: map[string]string{"status": "ready"}}
	}()

	started := time.Now()
	handler.command(recorder, request, RuntimeCommand{Kind: "mentor_outbox"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("normal cognition latency was exposed as mentor channel failure: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if time.Since(started) < 3*time.Second {
		t.Fatal("test worker did not exercise a delay beyond the former reply timeout")
	}
}
