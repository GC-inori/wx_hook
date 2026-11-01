package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"wxbot/pkg/config"
)

func TestPacerDoesNotDelayQueriesOrReadStatus(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:  true,
		MaxMessagesPerSec: 1,
		JitterMinMs:       150,
		JitterMaxMs:       150,
	})
	for _, action := range []string{"getSelfInfo", "getFriendList", "queryObj", "setReadStatus", "set_read_status"} {
		started := time.Now()
		if err := pacer.Pace(context.Background(), action, nil); err != nil {
			t.Fatalf("%s returned error: %v", action, err)
		}
		if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
			t.Errorf("%s was delayed by %s", action, elapsed)
		}
	}
}

func TestPacerEnforcesSameTargetInterval(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:    true,
		TargetMinIntervalMs: 120,
	})
	data1 := []byte(`{"wxid":"wxid_target","msg":"first"}`)
	data2 := []byte(`{"to_wxid":"wxid_target","content":"second"}`)
	if err := pacer.Pace(context.Background(), "sendText", data1); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := pacer.Pace(context.Background(), "send_text", data2); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("same-target send was paced for only %s", elapsed)
	}
}

func TestPacerGlobalRateLimitAcrossTargets(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:  true,
		MaxMessagesPerSec: 10,
	})
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{"wxid":"wx1"}`)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{"wxid":"wx2"}`)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 80*time.Millisecond {
		t.Fatalf("global rate limit allowed sends only %s apart", elapsed)
	}
}

func TestPacerDuplicateReturnsLocalizedHTTP429(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	s.pacer.UpdatePolicy(config.PolicyConfig{RateLimitEnabled: true, AntiLoopWindowSec: 2})
	body := `{"action":"sendText","params":{"wxid":"wxid_test","msg":"duplicate"}}`
	if status, response := request(t, s, "POST", body); status != 200 || response.Retcode != 0 {
		t.Fatalf("first send failed: status=%d response=%+v", status, response)
	}
	if status, response := request(t, s, "POST", body); status != 200 || response.Retcode != 1409 || response.Message != "Message throttled due to duplicate content or rate limit" {
		t.Fatalf("duplicate did not return localized HTTP 429: status=%d response=%+v", status, response)
	}
}

func TestPacerThrottlesDuplicateContent(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:  true,
		AntiLoopWindowSec: 2,
	})
	data := []byte(`{"chat_id":"room-1","msg":"exact content"}`)
	if err := pacer.Pace(context.Background(), "sendText", data); err != nil {
		t.Fatal(err)
	}
	if err := pacer.Pace(context.Background(), "sendText2", data); !errors.Is(err, ErrDuplicateThrottled) {
		t.Fatalf("duplicate returned %v, want ErrDuplicateThrottled", err)
	}
}

func TestPacerWaitHonorsContextCancellation(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:  true,
		MaxMessagesPerSec: 1,
	})
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := pacer.Pace(ctx, "sendText", []byte(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Pace returned %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("canceled pacing waited too long: %s", elapsed)
	}
}

func TestPacerUpdatesPolicyDynamically(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{})
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	pacer.UpdatePolicy(config.PolicyConfig{
		RateLimitEnabled: true,
		JitterMinMs:      60,
		JitterMaxMs:      60,
	})
	started := time.Now()
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 45*time.Millisecond {
		t.Fatalf("updated jitter policy was not applied, elapsed %s", elapsed)
	}
}

func TestPacerPerTargetIntervalDoesNotSuppressDifferentContentSpacing(t *testing.T) {
	pacer := NewOutboundPacer(config.PolicyConfig{
		RateLimitEnabled:    true,
		TargetMinIntervalMs: 40,
	})
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{"wxid":"wx1","msg":"same"}`)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := pacer.Pace(context.Background(), "sendText", []byte(`{"wxid":"wx1","msg":"same "}`)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 30*time.Millisecond {
		t.Fatalf("target interval was not enforced for changed content: %s", elapsed)
	}
}
