package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// tomo writes one canonical Session Trace Simple Format file per run: the first
// line is the session header and every later line is one logical message, the
// same self-describing artifact codex and claude emit. It is captured verbatim
// as session.jsonl beside the run's other trace files, so reconstructing a tomo
// run is a straight read of that one file rather than a reassembly of wire
// captures. This is the preferred path: it is what the agent actually recorded,
// already turn-structured, with no request/response stitching to get wrong.

// tomoMessages reconstructs a tomo run from its canonical STS session.jsonl.
// The second return is false when no such file is present, so the caller falls
// back to the wire-capture paths.
func tomoMessages(traceDir string) ([]Message, bool) {
	f, err := os.Open(filepath.Join(traceDir, "session.jsonl"))
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var msgs []Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var env stsEnvelope
		if json.Unmarshal(line, &env) != nil || env.Type != "message" {
			continue
		}
		msgs = append(msgs, stsMessage(env.Message))
	}
	return msgs, len(msgs) > 0
}

// stsEnvelope is one line of an STS file: a typed record whose "message" lines
// carry a turn. The session header line is a different type and is skipped.
type stsEnvelope struct {
	Type    string     `json:"type"`
	Message stsMsgWire `json:"message"`
}

// stsMsgWire is the message payload as the STS file carries it: a role, its
// content, its reasoning, and its tool calls, plus the tool_call_id a tool turn
// answers.
type stsMsgWire struct {
	Role             string        `json:"role"`
	Content          string        `json:"content"`
	ReasoningContent string        `json:"reasoningContent"`
	ToolCalls        []stsWireCall `json:"toolCalls"`
	ToolCallID       string        `json:"toolCallId"`
}

type stsWireCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// stsMessage maps one STS message to a Message. A tool turn is a single
// toolResult block; any other role is its reasoning, its text, and its tool
// calls, with the role preserved.
func stsMessage(m stsMsgWire) Message {
	if m.Role == "tool" {
		return Message{Role: "tool", Content: []Block{toolResultBlock(m.ToolCallID, m.Content)}}
	}
	msg := assistantMessage(m.Content, m.ReasoningContent, stsCalls(m.ToolCalls))
	msg.Role = m.Role
	return msg
}

func stsCalls(tcs []stsWireCall) []wireCall {
	out := make([]wireCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, wireCall{ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments})
	}
	return out
}
