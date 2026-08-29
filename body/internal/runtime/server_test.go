package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
