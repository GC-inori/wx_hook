package launcher

import (
	"testing"
)

func TestLoginManagerSnapshotActionsAndQRCode(t *testing.T) {
	manager := NewLoginManager(":99", "/tmp/wx_bridge.sock", "127.0.0.1:8080")
	status := manager.Snapshot()
	if status.Status != "launching" || status.Display != ":99" || status.Socket != "/tmp/wx_bridge.sock" || status.Listen != "127.0.0.1:8080" {
		t.Fatalf("unexpected initial state: %+v", status)
	}
	manager.Update("waiting_scan", "", 1234)
	manager.SetQRCode([]byte{1, 2, 3})
	qr, ok := manager.QRCode()
	if !ok || len(qr) != 3 {
		t.Fatalf("QR was not retained: %v %v", qr, ok)
	}
	qr[0] = 9
	copyQR, _ := manager.QRCode()
	if copyQR[0] != 1 {
		t.Fatal("QRCode did not return a defensive copy")
	}
	if manager.Snapshot().PID != 1234 {
		t.Fatal("process id was not recorded")
	}
	if err := manager.Refresh(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.RefreshRequests():
	default:
		t.Fatal("refresh request was not delivered")
	}
	if err := manager.Restart(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.RestartRequests():
	default:
		t.Fatal("restart request was not delivered")
	}
	if err := manager.Logout(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.LogoutRequests():
	default:
		t.Fatal("logout request was not delivered")
	}
}
