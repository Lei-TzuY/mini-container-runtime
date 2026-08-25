//go:build linux

package container

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func writeInitStatusPipe(t *testing.T, payload []byte) *os.File {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 0 {
		if _, err := writePipe.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writePipe.Close(); err != nil {
		t.Fatal(err)
	}
	return readPipe
}

func TestAwaitPayloadExecAcceptsReadyThenEOF(t *testing.T) {
	readPipe := writeInitStatusPipe(t, []byte{runtimeInitReadyByte})
	defer readPipe.Close()
	if err := awaitPayloadExec(readPipe); err != nil {
		t.Fatalf("ready + EOF rejected: %v", err)
	}
}

func TestAwaitPayloadExecRejectsEOFFailureAndMalformedStatus(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    string
	}{
		{name: "eof-before-ready", payload: nil, want: "payload exec was not confirmed"},
		{name: "failure-before-ready", payload: []byte{runtimeInitFailureByte}, want: "failed before payload exec"},
		{name: "malformed-first", payload: []byte{0x11}, want: "invalid runtime init status byte"},
		{name: "failure-after-ready", payload: []byte{runtimeInitReadyByte, runtimeInitFailureByte}, want: "failed while executing payload"},
		{name: "malformed-after-ready", payload: []byte{runtimeInitReadyByte, 0x22}, want: "invalid post-ready"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			readPipe := writeInitStatusPipe(t, tc.payload)
			defer readPipe.Close()
			err := awaitPayloadExec(readPipe)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("status error=%v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAwaitPayloadExecRejectsNilReader(t *testing.T) {
	if err := awaitPayloadExec(nil); err == nil {
		t.Fatal("nil status reader accepted")
	}
}

func TestRuntimeInitStatusWriterReadyThenCloseModelsSuccessfulExec(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer := &runtimeInitStatusWriter{file: writePipe}
	if err := writer.readyForExec(); err != nil {
		t.Fatalf("readyForExec: %v", err)
	}
	writer.finish(nil)
	defer readPipe.Close()
	if err := awaitPayloadExec(readPipe); err != nil {
		t.Fatalf("successful boundary rejected: %v", err)
	}
}

func TestRuntimeInitStatusWriterFailureBeforeReady(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer := &runtimeInitStatusWriter{file: writePipe}
	writer.finish(errors.New("mount failed"))
	defer readPipe.Close()
	err = awaitPayloadExec(readPipe)
	if err == nil || !strings.Contains(err.Error(), "failed before payload exec") {
		t.Fatalf("failure status=%v", err)
	}
}

func TestRuntimeInitStatusWriterFailureAfterReady(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer := &runtimeInitStatusWriter{file: writePipe}
	if err := writer.readyForExec(); err != nil {
		t.Fatalf("readyForExec: %v", err)
	}
	writer.finish(errors.New("exec failed"))
	defer readPipe.Close()
	err = awaitPayloadExec(readPipe)
	if err == nil || !strings.Contains(err.Error(), "failed while executing payload") {
		t.Fatalf("post-ready failure status=%v", err)
	}
}

func TestRuntimeInitStatusWriterRejectsNilWriter(t *testing.T) {
	var writer *runtimeInitStatusWriter
	if err := writer.readyForExec(); err == nil {
		t.Fatal("nil status writer accepted")
	}
}

func TestAwaitPayloadExecPreservesUnexpectedReadError(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatal(err)
	}
	defer writePipe.Close()
	err = awaitPayloadExec(readPipe)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("closed-reader error=%v", err)
	}
}

func TestJoinRuntimeInitFailurePreservesExistingErrorAndBlocksRestart(t *testing.T) {
	payloadErr := errors.New("child exit")
	initErr := errors.New("mount namespace failed")
	got := joinRuntimeInitFailure(payloadErr, initErr)
	if !errors.Is(got, payloadErr) || !errors.Is(got, initErr) {
		t.Fatalf("joined init result=%v", got)
	}
	if !isRuntimeControlError(got) {
		t.Fatalf("init failure not classified as runtime control: %v", got)
	}
}

func TestJoinRuntimeInitFailureNoopOnSuccess(t *testing.T) {
	payloadErr := errors.New("payload exit")
	if got := joinRuntimeInitFailure(payloadErr, nil); got != payloadErr {
		t.Fatalf("nil init error changed result: %v", got)
	}
}
