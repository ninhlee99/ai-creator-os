package products

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTryOnTestStore(t *testing.T, count int) (*Store, []int64) {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetBioProfile(1, true, "", true); err != nil {
		t.Fatal(err)
	}
	productIDs := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		id, err := store.Save(Product{Source: "manual", SourceID: fmt.Sprintf("tryon-%d", i), Title: "Áo thử"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetBioListing(1, id, true, i, "", 0, "", ""); err != nil {
			t.Fatal(err)
		}
		productIDs = append(productIDs, id)
	}
	return store, productIDs
}

func tryOnRequest(productID int64, ip, device, image string, n int, now time.Time) BioTryOnRequest {
	return BioTryOnRequest{
		ID: fmt.Sprintf("bio-job-%024d", n), AccountID: 1, ProductID: productID,
		IPHash: ip, DeviceHash: device, ImageHash: image, Now: now,
	}
}

func TestReserveBioTryOnLimitsProductAndReusesIdenticalInput(t *testing.T) {
	store, ids := newTryOnTestStore(t, 2)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	first := tryOnRequest(ids[0], "ip-a", "device-a", "photo-a", 1, now)
	reserved, err := store.ReserveBioTryOn(first)
	if err != nil || reserved.Job.ID != first.ID || reserved.Reused || reserved.QuotaReason != "" {
		t.Fatalf("first reservation = %+v, err=%v", reserved, err)
	}
	if err := store.SetBioTryOnJobState(first.ID, "succeeded", "opaque/result.webp", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	duplicate := first
	duplicate.ID = "bio-job-duplicate-00000001"
	duplicate.Now = now.Add(2 * time.Second)
	reused, err := store.ReserveBioTryOn(duplicate)
	if err != nil || !reused.Reused || reused.Job.ID != first.ID || reused.Job.Status != "succeeded" {
		t.Fatalf("duplicate input did not reuse result: %+v err=%v", reused, err)
	}
	duplicate.ID = "bio-job-network-change-0001"
	duplicate.IPHash = "ip-b"
	duplicate.Now = now.Add(3 * time.Second)
	reused, err = store.ReserveBioTryOn(duplicate)
	if err != nil || !reused.Reused || reused.Job.ID != first.ID {
		t.Fatalf("same device on a new IP did not reuse result: %+v err=%v", reused, err)
	}

	for i, image := range []string{"photo-b", "photo-c"} {
		request := tryOnRequest(ids[0], "ip-a", "device-a", image, i+2, now)
		if result, err := store.ReserveBioTryOn(request); err != nil || result.Job.ID != request.ID {
			t.Fatalf("product attempt %d = %+v, err=%v", i+2, result, err)
		}
	}
	fourth := tryOnRequest(ids[0], "ip-a", "device-a", "photo-d", 4, now)
	if result, err := store.ReserveBioTryOn(fourth); err != nil || result.QuotaReason != "ip_product" {
		t.Fatalf("fourth IP/product attempt should stop: %+v err=%v", result, err)
	}
	// Device quota remains independently enforced after the IP changes.
	newIP := tryOnRequest(ids[0], "ip-b", "device-a", "photo-e", 5, now)
	if result, err := store.ReserveBioTryOn(newIP); err != nil || result.QuotaReason != "device_product" {
		t.Fatalf("fourth device/product attempt should stop: %+v err=%v", result, err)
	}
}

func TestReserveBioTryOnDailyLimitAcrossProducts(t *testing.T) {
	store, ids := newTryOnTestStore(t, 6)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		product := ids[i/3]
		request := tryOnRequest(product, "ip-daily", "device-daily", fmt.Sprintf("photo-%d", i), i+1, now)
		if result, err := store.ReserveBioTryOn(request); err != nil || result.Job.ID != request.ID {
			t.Fatalf("daily attempt %d = %+v, err=%v", i+1, result, err)
		}
	}
	request := tryOnRequest(ids[5], "ip-daily", "device-daily", "photo-16", 16, now)
	if result, err := store.ReserveBioTryOn(request); err != nil || result.QuotaReason != "ip_daily" {
		t.Fatalf("16th daily request should stop: %+v err=%v", result, err)
	}

	// A new local calendar day opens daily capacity, while lifetime product caps remain.
	newDay := tryOnRequest(ids[5], "ip-daily", "device-daily", "photo-next-day", 17, now.Add(24*time.Hour))
	if result, err := store.ReserveBioTryOn(newDay); err != nil || result.Job.ID != newDay.ID {
		t.Fatalf("next day should reset daily cap: %+v err=%v", result, err)
	}
}

func TestReserveBioTryOnConcurrentRequestsCannotOverrunQuota(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := tryOnRequest(ids[0], "ip-race", fmt.Sprintf("device-%d", i), fmt.Sprintf("photo-%d", i), i+1, now)
			result, err := store.ReserveBioTryOn(request)
			if err != nil {
				t.Errorf("ReserveBioTryOn: %v", err)
				return
			}
			if result.Job.ID != "" {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if accepted != BioTryOnProductLimit {
		t.Fatalf("accepted %d concurrent generations, want %d", accepted, BioTryOnProductLimit)
	}
}

func TestReserveBioTryOnConcurrentDuplicateSubmissionsShareOneJob(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var mu sync.Mutex
	jobIDs := make(map[string]bool)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := tryOnRequest(ids[0], "ip-duplicate", "device-duplicate", "same-photo", i+1, now)
			result, err := store.ReserveBioTryOn(request)
			if err != nil {
				t.Errorf("ReserveBioTryOn duplicate: %v", err)
				return
			}
			if result.Job.ID == "" {
				t.Errorf("duplicate request did not reserve or reuse job: %+v", result)
				return
			}
			mu.Lock()
			jobIDs[result.Job.ID] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(jobIDs) != 1 {
		t.Fatalf("duplicate requests created %d jobs, want 1: %v", len(jobIDs), jobIDs)
	}

	for i, image := range []string{"photo-b", "photo-c"} {
		request := tryOnRequest(ids[0], "ip-duplicate", "device-duplicate", image, i+20, now)
		if result, err := store.ReserveBioTryOn(request); err != nil || result.Job.ID != request.ID {
			t.Fatalf("unique request %d after duplicate submissions = %+v err=%v", i+1, result, err)
		}
	}
	request := tryOnRequest(ids[0], "ip-duplicate", "device-duplicate", "photo-d", 30, now)
	if result, err := store.ReserveBioTryOn(request); err != nil || result.QuotaReason != "ip_product" {
		t.Fatalf("duplicate submissions must consume one slot: %+v err=%v", result, err)
	}
}

func TestGetBioTryOnJobRequiresMatchingIdentityAndRetention(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	request := tryOnRequest(ids[0], "ip-read", "device-read", "photo-read", 1, now)
	if _, err := store.ReserveBioTryOn(request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetBioTryOnJob(request.ID, "different-device", now); err != ErrBioTryOnJobNotFound {
		t.Fatalf("unrelated identity read job: err=%v", err)
	}
	job, err := store.GetBioTryOnJob(request.ID, "device-read", now)
	if err != nil || job.Status != "queued" {
		t.Fatalf("same device on a new IP should read job: job=%+v err=%v", job, err)
	}
	if _, err := store.GetBioTryOnJob(request.ID, "device-read", now.Add(BioTryOnRetention)); err != ErrBioTryOnJobNotFound {
		t.Fatalf("expired job should not be retrievable: err=%v", err)
	}
}

func TestBioTryOnReuseDoesNotCrossSharedIP(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	first := tryOnRequest(ids[0], "ip-shared", "device-one", "same-photo", 1, now)
	if _, err := store.ReserveBioTryOn(first); err != nil {
		t.Fatal(err)
	}
	second := tryOnRequest(ids[0], "ip-shared", "device-two", "same-photo", 2, now.Add(time.Second))
	result, err := store.ReserveBioTryOn(second)
	if err != nil || result.Reused || result.Job.ID != second.ID {
		t.Fatalf("same IP must not reuse another device's upload: result=%+v err=%v", result, err)
	}
	if _, err := store.GetBioTryOnJob(first.ID, "device-two", now); err != ErrBioTryOnJobNotFound {
		t.Fatalf("same IP device should not read another device's result: err=%v", err)
	}
}

func TestBioTryOnExpiredJobCanBeGeneratedAgain(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	first := tryOnRequest(ids[0], "ip-expiry", "device-expiry", "photo-expiry", 1, now)
	if _, err := store.ReserveBioTryOn(first); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioTryOnJobState(first.ID, "succeeded", "opaque/result.webp", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	second := tryOnRequest(ids[0], "ip-expiry", "device-expiry", "photo-expiry", 2,
		now.Add(time.Second+BioTryOnRetention))
	result, err := store.ReserveBioTryOn(second)
	if err != nil || result.Reused || result.Job.ID != second.ID {
		t.Fatalf("expired result should stop reusing and reserve anew: result=%+v err=%v", result, err)
	}
}

func TestRunningBioTryOnGetsProcessingDeadline(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	request := tryOnRequest(ids[0], "ip-running", "device-running", "photo-running", 1, now)
	if _, err := store.ReserveBioTryOn(request); err != nil {
		t.Fatal(err)
	}
	started := now.Add(10 * time.Minute)
	if err := store.SetBioTryOnJobState(request.ID, "running", "", "", started); err != nil {
		t.Fatal(err)
	}
	job, err := store.GetBioTryOnJob(request.ID, "device-running", started.Add(20*time.Minute))
	if err != nil || job.Status != "running" || !job.ExpiresAt.Equal(started.Add(BioTryOnProcessingTimeout)) {
		t.Fatalf("running job should outlive queue timeout: job=%+v err=%v", job, err)
	}
}

func TestCleanupExpiredBioTryOnResultsSweepsOnlyOldOrphans(t *testing.T) {
	store, ids := newTryOnTestStore(t, 1)
	now := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	dir := filepath.Join(t.TempDir(), BioTryOnResultsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	active := tryOnRequest(ids[0], "ip-active", "device-active", "photo-active", 1, now)
	if _, err := store.ReserveBioTryOn(active); err != nil {
		t.Fatal(err)
	}
	paths := map[string]time.Time{
		filepath.Join(dir, "orphan-job-0000000000000000.webp"): now.Add(-BioTryOnProcessingTimeout - BioTryOnQueueTimeout - time.Minute),
		filepath.Join(dir, active.ID+".webp"):                  now.Add(-BioTryOnProcessingTimeout - BioTryOnQueueTimeout - time.Minute),
		filepath.Join(dir, "recent-job-0000000000000000.webp"): now.Add(-time.Hour),
		filepath.Join(dir, "notes.txt"):                        now.Add(-24 * time.Hour),
	}
	for path, modifiedAt := range paths {
		if err := os.WriteFile(path, []byte("result"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := store.CleanupExpiredBioTryOnResults(dir, now)
	if err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v, want only one orphan", removed, err)
	}
	for path, modifiedAt := range paths {
		_, statErr := os.Stat(path)
		wantExists := path != filepath.Join(dir, "orphan-job-0000000000000000.webp")
		if wantExists && statErr != nil {
			t.Errorf("cleanup removed protected file %q (mtime %s): %v", filepath.Base(path), modifiedAt, statErr)
		}
		if !wantExists && !os.IsNotExist(statErr) {
			t.Errorf("cleanup retained old orphan %q: %v", filepath.Base(path), statErr)
		}
	}
}
