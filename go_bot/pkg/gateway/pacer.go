package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"wxbot/pkg/config"
	"wxbot/pkg/protocol"
)

// ErrDuplicateThrottled reports a repeated outgoing message that falls within
// the configured anti-loop window.
var ErrDuplicateThrottled = errors.New("duplicate message throttled")

var outboundMessageActions = map[string]struct{}{
	"sendText": {}, "sendText2": {}, "sendXml": {}, "sendImage": {}, "sendFile": {},
	"sendGif": {}, "sendCard": {}, "sendShareUrl": {}, "sendApplet": {}, "sendReferText": {},
}

type duplicateRecord struct {
	at      time.Time
	pending bool
	token   uint64
}

// OutboundPacer applies rate, target-spacing, jitter, and duplicate controls to
// outgoing messages. Read-only and other non-message actions pass through.
type OutboundPacer struct {
	mu          sync.Mutex
	policy      config.PolicyConfig
	lastTarget  map[string]time.Time
	duplicates  map[string]duplicateRecord
	nextGlobal  time.Time
	lastCleanup time.Time
	reservation uint64
}

func NewOutboundPacer(policy config.PolicyConfig) *OutboundPacer {
	return &OutboundPacer{
		policy:     policy,
		lastTarget: make(map[string]time.Time),
		duplicates: make(map[string]duplicateRecord),
	}
}

// UpdatePolicy atomically replaces the pacing policy for subsequent messages.
func (p *OutboundPacer) UpdatePolicy(policy config.PolicyConfig) {
	p.mu.Lock()
	p.policy = policy
	p.mu.Unlock()
}

// Pace waits until an outgoing message is allowed. Non-message actions are
// returned immediately, so queries and read-status calls are never delayed.
func (p *OutboundPacer) Pace(ctx context.Context, action string, data any) error {
	canonical := action
	if name, ok := protocol.Canonical(action); ok {
		canonical = name
	}
	if _, outbound := outboundMessageActions[canonical]; !outbound {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	target, content := messageIdentity(data)
	now := time.Now()
	p.mu.Lock()
	policy := p.policy
	if !policy.RateLimitEnabled {
		p.mu.Unlock()
		return nil
	}
	p.cleanupLocked(now, policy)

	var duplicateKey string
	var duplicateToken uint64
	antiLoopWindow := seconds(policy.AntiLoopWindowSec)
	if target != "" && content != "" && antiLoopWindow > 0 {
		duplicateKey = target + "\x00" + content
		if previous, ok := p.duplicates[duplicateKey]; ok {
			if previous.pending || now.Sub(previous.at) < antiLoopWindow {
				p.mu.Unlock()
				return ErrDuplicateThrottled
			}
			delete(p.duplicates, duplicateKey)
		}
		p.reservation++
		duplicateToken = p.reservation
		p.duplicates[duplicateKey] = duplicateRecord{at: now, pending: true, token: duplicateToken}
	}

	eligibleAt := now
	if target != "" {
		if previous := p.lastTarget[target]; !previous.IsZero() {
			eligibleAt = laterTime(eligibleAt, previous.Add(milliseconds(policy.TargetMinIntervalMs)))
		}
	}
	interval := messagesPerSecondInterval(policy.MaxMessagesPerSec)
	if interval > 0 && !p.nextGlobal.IsZero() {
		eligibleAt = laterTime(eligibleAt, p.nextGlobal)
	}

	jitter := randomizedJitter(policy.JitterMinMs, policy.JitterMaxMs)
	scheduledAt := eligibleAt.Add(jitter)
	if target != "" {
		p.lastTarget[target] = scheduledAt
	}
	if interval > 0 {
		p.nextGlobal = scheduledAt.Add(interval)
	}
	p.mu.Unlock()

	if err := waitUntil(ctx, scheduledAt); err != nil {
		p.clearPendingDuplicate(duplicateKey, duplicateToken)
		return err
	}

	if duplicateKey != "" {
		p.mu.Lock()
		if entry, ok := p.duplicates[duplicateKey]; ok && entry.token == duplicateToken {
			entry.at = time.Now()
			entry.pending = false
			p.duplicates[duplicateKey] = entry
		}
		p.mu.Unlock()
	}
	return nil
}

func (p *OutboundPacer) clearPendingDuplicate(key string, token uint64) {
	if key == "" {
		return
	}
	p.mu.Lock()
	if entry, ok := p.duplicates[key]; ok && entry.pending && entry.token == token {
		delete(p.duplicates, key)
	}
	p.mu.Unlock()
}

func (p *OutboundPacer) cleanupLocked(now time.Time, policy config.PolicyConfig) {
	if !p.lastCleanup.IsZero() && now.Sub(p.lastCleanup) < time.Minute {
		return
	}
	p.lastCleanup = now

	window := seconds(policy.AntiLoopWindowSec)
	for key, entry := range p.duplicates {
		if !entry.pending && (window <= 0 || now.Sub(entry.at) >= window) {
			delete(p.duplicates, key)
		}
	}

	staleAfter := time.Minute
	if targetInterval := milliseconds(policy.TargetMinIntervalMs); targetInterval > staleAfter {
		staleAfter = targetInterval
	}
	if window > staleAfter {
		staleAfter = window
	}
	for target, sentAt := range p.lastTarget {
		if now.Sub(sentAt) > staleAfter {
			delete(p.lastTarget, target)
		}
	}
}

func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func milliseconds(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	const maxMillis = int64((1<<63 - 1) / int64(time.Millisecond))
	millis := int64(value)
	if millis > maxMillis {
		millis = maxMillis
	}
	return time.Duration(millis) * time.Millisecond
}

func seconds(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
	seconds := int64(value)
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	return time.Duration(seconds) * time.Second
}

func messagesPerSecondInterval(maxMessages int) time.Duration {
	if maxMessages <= 0 {
		return 0
	}
	perSecond := time.Duration(maxMessages)
	interval := time.Second / perSecond
	if time.Second%perSecond != 0 {
		interval++
	}
	if interval <= 0 {
		return time.Nanosecond
	}
	return interval
}

func randomizedJitter(minMs, maxMs int) time.Duration {
	if maxMs <= 0 {
		return 0
	}
	if minMs < 0 {
		minMs = 0
	}
	if maxMs < minMs {
		maxMs = minMs
	}
	minimum := milliseconds(minMs)
	maximum := milliseconds(maxMs)
	spreadMs := int64((maximum - minimum) / time.Millisecond)
	if spreadMs == 0 {
		return minimum
	}
	return minimum + time.Duration(rand.Int63n(spreadMs+1))*time.Millisecond
}

func waitUntil(ctx context.Context, until time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := time.Until(until)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func messageIdentity(data any) (target, content string) {
	var raw []byte
	switch value := data.(type) {
	case json.RawMessage:
		raw = value
	case []byte:
		raw = value
	default:
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return "", ""
		}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", ""
	}
	for _, key := range []string{"wxid", "to_wxid", "target", "chat_id", "receiver"} {
		if value := stringField(fields[key]); value != "" {
			target = value
			break
		}
	}
	for _, key := range []string{"msg", "content", "text"} {
		if value := contentField(fields[key]); value != "" {
			content = value
			break
		}
	}
	return target, content
}

func contentField(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func stringField(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
