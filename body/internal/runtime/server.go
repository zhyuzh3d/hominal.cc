package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const mentorSocketPath = "/run/hominal/hominal.sock"

const (
	mentorCommandEnqueueTimeout = 3 * time.Second
	mentorCommandReplyTimeout   = 10 * time.Second
)

var messageIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type MentorServer struct {
	server   *http.Server
	listener net.Listener
	commands chan<- RuntimeCommand
}

func StartMentorServer(commands chan<- RuntimeCommand) (*MentorServer, error) {
	directory := filepath.Dir(mentorSocketPath)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	if err := setRuntimeDirectoryAccess(directory); err != nil {
		return nil, err
	}
	if err := os.Remove(mentorSocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", mentorSocketPath)
	if err != nil {
		return nil, err
	}
	if err := setSocketAccess(mentorSocketPath); err != nil {
		listener.Close()
		return nil, err
	}
	handler := &mentorHandler{commands: commands}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      12 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	result := &MentorServer{server: server, listener: listener, commands: commands}
	go func() {
		_ = server.Serve(listener)
	}()
	return result, nil
}

func (s *MentorServer) Close(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	_ = s.listener.Close()
	_ = os.Remove(mentorSocketPath)
	return err
}

type mentorHandler struct {
	commands chan<- RuntimeCommand
}

func (h *mentorHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/mentor/inbox":
		h.receive(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/environment/events":
		h.receiveEnvironment(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/v1/lab/deadline":
		h.receiveDeadline(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/mentor/outbox":
		h.command(writer, request, RuntimeCommand{Kind: "mentor_outbox"})
	case request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/v1/mentor/outbox/") && strings.HasSuffix(request.URL.Path, "/ack"):
		messageID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v1/mentor/outbox/"), "/ack")
		if !messageIDPattern.MatchString(messageID) {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid message id"})
			return
		}
		h.command(writer, request, RuntimeCommand{Kind: "mentor_ack", MessageID: messageID})
	default:
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *mentorHandler) receiveDeadline(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 4*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input GenerationDeadlineInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input.PlannedEnd = strings.TrimSpace(input.PlannedEnd)
	if input.PlannedEnd == "" || len(input.PlannedEnd) > 64 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid planned_end"})
		return
	}
	h.command(writer, request, RuntimeCommand{Kind: "generation_extend", Deadline: input})
}

func (h *mentorHandler) receiveEnvironment(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input EnvironmentInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input.Summary = strings.TrimSpace(input.Summary)
	if !messageIDPattern.MatchString(input.EventID) || input.Summary == "" || len(input.Summary) > 4096 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid environment event"})
		return
	}
	if len(input.Payload) > 56*1024 || (len(input.Payload) > 0 && !json.Valid(input.Payload)) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid environment payload"})
		return
	}
	h.command(writer, request, RuntimeCommand{Kind: "environment_receive", Environment: input})
}

func (h *mentorHandler) receive(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input MentorInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if !messageIDPattern.MatchString(input.MessageID) || input.Body == "" || len(input.Body) > 60*1024 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid message"})
		return
	}
	if input.ReplyTo != "" && !messageIDPattern.MatchString(input.ReplyTo) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid reply_to"})
		return
	}
	h.command(writer, request, RuntimeCommand{Kind: "mentor_receive", Mentor: input})
}

func (h *mentorHandler) command(writer http.ResponseWriter, request *http.Request, command RuntimeCommand) {
	command.Reply = make(chan CommandReply, 1)
	select {
	case h.commands <- command:
	case <-request.Context().Done():
		writeJSON(writer, http.StatusRequestTimeout, map[string]string{"error": "request cancelled"})
		return
	case <-time.After(mentorCommandEnqueueTimeout):
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "runtime busy"})
		return
	}
	select {
	case reply := <-command.Reply:
		writeJSON(writer, reply.Status, reply.Body)
	case <-request.Context().Done():
		writeJSON(writer, http.StatusRequestTimeout, map[string]string{"error": "request cancelled"})
	case <-time.After(mentorCommandReplyTimeout):
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "runtime did not reply"})
	}
}

func setSocketAccess(path string) error {
	gid, err := hominalGroupID()
	if err != nil {
		return err
	}
	if err := os.Chown(path, 0, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0o660)
}

func setRuntimeDirectoryAccess(path string) error {
	gid, err := hominalGroupID()
	if err != nil {
		return err
	}
	if err := os.Chown(path, 0, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0o750)
}

func hominalGroupID() (int, error) {
	account, err := user.Lookup("hominal")
	if err != nil {
		return 0, fmt.Errorf("lookup hominal user: %w", err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, err
	}
	return gid, nil
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
