package ptyserver

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

type perfStats struct {
	ptyFlushes             atomic.Uint64
	ptyBytes               atomic.Uint64
	ptyMaxFlushBytes       atomic.Uint64
	wsWrites               atomic.Uint64
	wsBytes                atomic.Uint64
	wsWriteErrors          atomic.Uint64
	wsWriteTimeouts        atomic.Uint64
	wsWriteMaxNanos        atomic.Uint64
	wsSlowClientDrops      atomic.Uint64
	screenDrains           atomic.Uint64
	screenDrainBytes       atomic.Uint64
	screenDrainMaxNanos    atomic.Uint64
	snapshotRenders        atomic.Uint64
	snapshotBytes          atomic.Uint64
	snapshotRenderMaxNanos atomic.Uint64
	scrollbackWrites       atomic.Uint64
	scrollbackBytes        atomic.Uint64
	scrollbackMaxNanos     atomic.Uint64
}

type perfSnapshot struct {
	PTYFlushes           uint64 `json:"pty_flushes"`
	PTYBytes             uint64 `json:"pty_bytes"`
	PTYMaxFlushBytes     uint64 `json:"pty_max_flush_bytes"`
	WSWrites             uint64 `json:"ws_writes"`
	WSBytes              uint64 `json:"ws_bytes"`
	WSWriteErrors        uint64 `json:"ws_write_errors"`
	WSWriteTimeouts      uint64 `json:"ws_write_timeouts"`
	WSWriteMaxMs         uint64 `json:"ws_write_max_ms"`
	WSSlowClientDrops    uint64 `json:"ws_slow_client_drops"`
	ScreenDrains         uint64 `json:"screen_drains"`
	ScreenDrainBytes     uint64 `json:"screen_drain_bytes"`
	ScreenDrainMaxMs     uint64 `json:"screen_drain_max_ms"`
	SnapshotRenders      uint64 `json:"snapshot_renders"`
	SnapshotBytes        uint64 `json:"snapshot_bytes"`
	SnapshotRenderMaxMs  uint64 `json:"snapshot_render_max_ms"`
	ScrollbackWrites     uint64 `json:"scrollback_writes"`
	ScrollbackBytes      uint64 `json:"scrollback_bytes"`
	ScrollbackWriteMaxMs uint64 `json:"scrollback_write_max_ms"`
}

func observeMax(dst *atomic.Uint64, value uint64) {
	for {
		old := dst.Load()
		if value <= old {
			return
		}
		if dst.CompareAndSwap(old, value) {
			return
		}
	}
}

func durationMillis(nanos uint64) uint64 {
	return nanos / uint64(time.Millisecond)
}

func (p *perfStats) observePTYFlush(bytes int) {
	p.ptyFlushes.Add(1)
	p.ptyBytes.Add(uint64(bytes))
	observeMax(&p.ptyMaxFlushBytes, uint64(bytes))
}

func (p *perfStats) observeWSWrite(bytes int, dur time.Duration, err error) {
	p.wsWrites.Add(1)
	p.wsBytes.Add(uint64(bytes))
	observeMax(&p.wsWriteMaxNanos, uint64(dur))
	if err == nil {
		return
	}
	p.wsWriteErrors.Add(1)
	if errors.Is(err, context.DeadlineExceeded) {
		p.wsWriteTimeouts.Add(1)
	}
}

func (p *perfStats) observeSlowClientDrop() {
	p.wsSlowClientDrops.Add(1)
}

func (p *perfStats) observeScreenDrain(bytes int, dur time.Duration) {
	p.screenDrains.Add(1)
	p.screenDrainBytes.Add(uint64(bytes))
	observeMax(&p.screenDrainMaxNanos, uint64(dur))
}

func (p *perfStats) observeSnapshotRender(bytes int, dur time.Duration) {
	p.snapshotRenders.Add(1)
	p.snapshotBytes.Add(uint64(bytes))
	observeMax(&p.snapshotRenderMaxNanos, uint64(dur))
}

func (p *perfStats) observeScrollbackWrite(bytes int, dur time.Duration) {
	p.scrollbackWrites.Add(1)
	p.scrollbackBytes.Add(uint64(bytes))
	observeMax(&p.scrollbackMaxNanos, uint64(dur))
}

func (p *perfStats) snapshot() perfSnapshot {
	return perfSnapshot{
		PTYFlushes:           p.ptyFlushes.Load(),
		PTYBytes:             p.ptyBytes.Load(),
		PTYMaxFlushBytes:     p.ptyMaxFlushBytes.Load(),
		WSWrites:             p.wsWrites.Load(),
		WSBytes:              p.wsBytes.Load(),
		WSWriteErrors:        p.wsWriteErrors.Load(),
		WSWriteTimeouts:      p.wsWriteTimeouts.Load(),
		WSWriteMaxMs:         durationMillis(p.wsWriteMaxNanos.Load()),
		WSSlowClientDrops:    p.wsSlowClientDrops.Load(),
		ScreenDrains:         p.screenDrains.Load(),
		ScreenDrainBytes:     p.screenDrainBytes.Load(),
		ScreenDrainMaxMs:     durationMillis(p.screenDrainMaxNanos.Load()),
		SnapshotRenders:      p.snapshotRenders.Load(),
		SnapshotBytes:        p.snapshotBytes.Load(),
		SnapshotRenderMaxMs:  durationMillis(p.snapshotRenderMaxNanos.Load()),
		ScrollbackWrites:     p.scrollbackWrites.Load(),
		ScrollbackBytes:      p.scrollbackBytes.Load(),
		ScrollbackWriteMaxMs: durationMillis(p.scrollbackMaxNanos.Load()),
	}
}
