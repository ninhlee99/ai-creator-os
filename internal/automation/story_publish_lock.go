package automation

import "sync"

type publishAttemptLock struct {
	mu   sync.Mutex
	refs int
}

var publishAttemptLockRegistry = struct {
	sync.Mutex
	locks map[string]*publishAttemptLock
}{locks: make(map[string]*publishAttemptLock)}

// tryStoryPublishLock prevents concurrent requests in this process from
// starting another upload for the same Story job.
func tryStoryPublishLock(jobID string) (func(), bool) {
	return tryPublishAttemptLock("story\x00" + jobID)
}

func tryAffiliatePublishLock(jobID, platform string) (func(), bool) {
	return tryPublishAttemptLock("affiliate\x00" + jobID + "\x00" + platform)
}

func tryPublishAttemptLock(key string) (func(), bool) {
	publishAttemptLockRegistry.Lock()
	lock := publishAttemptLockRegistry.locks[key]
	if lock == nil {
		lock = &publishAttemptLock{}
		publishAttemptLockRegistry.locks[key] = lock
	}
	lock.refs++
	publishAttemptLockRegistry.Unlock()

	if !lock.mu.TryLock() {
		releasePublishAttemptLock(key, lock)
		return nil, false
	}
	return func() {
		lock.mu.Unlock()
		releasePublishAttemptLock(key, lock)
	}, true
}

func releasePublishAttemptLock(key string, lock *publishAttemptLock) {
	publishAttemptLockRegistry.Lock()
	defer publishAttemptLockRegistry.Unlock()
	lock.refs--
	if lock.refs == 0 {
		delete(publishAttemptLockRegistry.locks, key)
	}
}
