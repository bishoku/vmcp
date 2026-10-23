package delivery

import "testing"

func TestLocalWakeIsBoundedSessionScopedAndReplicaLocal(t *testing.T) {
	local, remote := &Store{}, &Store{}
	wake, unsubscribe := local.Subscribe("session-a")
	defer unsubscribe()
	other, closeOther := local.Subscribe("session-b")
	defer closeOther()
	remoteWake, closeRemote := remote.Subscribe("session-a")
	defer closeRemote()
	for i := 0; i < 100; i++ {
		local.NotifyLocal("session-a")
	}
	select {
	case <-wake:
	default:
		t.Fatal("same-replica reader was not woken")
	}
	select {
	case <-wake:
		t.Fatal("wakeups were not coalesced")
	default:
	}
	select {
	case <-other:
		t.Fatal("unrelated session woken")
	default:
	}
	select {
	case <-remoteWake:
		t.Fatal("another replica received a local wake")
	default:
	}
	unsubscribe()
	local.NotifyLocal("session-a")
	select {
	case <-wake:
		t.Fatal("unsubscribed reader woken")
	default:
	}
}
