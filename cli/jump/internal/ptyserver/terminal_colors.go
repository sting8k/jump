package ptyserver

import "bytes"

const (
	oscBEL = byte(0x07)
	esc    = byte(0x1b)
)

type terminalColorTracker struct {
	pending []byte
	bgSeq   string
}

func (t *terminalColorTracker) write(data []byte) {
	if len(data) == 0 {
		return
	}

	combined := data
	if len(t.pending) > 0 {
		combined = make([]byte, 0, len(t.pending)+len(data))
		combined = append(combined, t.pending...)
		combined = append(combined, data...)
		t.pending = nil
	}

	for i := 0; i < len(combined); {
		payloadStart, ok := oscPayloadStart(combined, i)
		if !ok {
			i++
			continue
		}

		payloadEnd, seqEnd, found := oscTerminator(combined, payloadStart)
		if !found {
			t.pending = append(t.pending[:0], combined[i:]...)
			return
		}

		t.applyOSC(combined[payloadStart:payloadEnd])
		i = seqEnd
	}

	if len(combined) > 0 && combined[len(combined)-1] == esc {
		t.pending = append(t.pending[:0], esc)
	}
}

func (t *terminalColorTracker) backgroundReplaySeq() string {
	if t.bgSeq != "" {
		return t.bgSeq
	}
	return "\x1b]111\x07"
}

func (t *terminalColorTracker) applyOSC(payload []byte) {
	sep := bytes.IndexByte(payload, ';')
	id := payload
	value := []byte(nil)
	if sep >= 0 {
		id = payload[:sep]
		value = payload[sep+1:]
	}

	switch string(id) {
	case "11":
		if sep < 0 || len(value) == 0 || bytes.Equal(value, []byte("?")) {
			return
		}
		t.bgSeq = "\x1b]11;" + string(value) + "\x07"
	case "111":
		t.bgSeq = ""
	}
}

func oscPayloadStart(data []byte, i int) (int, bool) {
	if data[i] == 0x9d {
		return i + 1, true
	}
	if data[i] == esc && i+1 < len(data) && data[i+1] == ']' {
		return i + 2, true
	}
	return 0, false
}

func oscTerminator(data []byte, start int) (payloadEnd int, seqEnd int, ok bool) {
	for i := start; i < len(data); i++ {
		if data[i] == oscBEL {
			return i, i + 1, true
		}
		if data[i] == esc && i+1 < len(data) && data[i+1] == '\\' {
			return i, i + 2, true
		}
	}
	return 0, 0, false
}
