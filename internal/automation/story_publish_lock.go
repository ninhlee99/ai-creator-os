package automation

import "sync"

type storyPublishLock struct {
	mu   sync.Mutex
	refs int
}

var storyPublishLockRegistry = struct {
	sync.Mutex
	locks map[string]*storyPublishLock
}{locks: make(map[string]*storyPublishLock)}

// tryStoryPublishLock prevents concurrent requests in this process from
// starting another upload for the same Story job.
func tryStoryPublishLock(jobID string) (func(), bool) {
	storyPublishLockRegistry.Lock()
	lock := storyPublishLockRegistry.locks[jobID]
	if lock == nil {
		lock = &storyPublishLock{}
		storyPublishLockRegistry.locks[jobID] = lock
	}
	lock.refs++
	storyPublishLockRegistry.Unlock()

	if !lock.mu.TryLock() {
		releaseStoryPublishLock(jobID, lock)
		return nil, false
	}
	return func() {
		lock.mu.Unlock()
		releaseStoryPublishLock(jobID, lock)
	}, true
}

func releaseStoryPublishLock(jobID string, lock *storyPublishLock) {
	storyPublishLockRegistry.Lock()
	defer storyPublishLockRegistry.Unlock()
	lock.refs--
	if lock.refs == 0 {
		delete(storyPublishLockRegistry.locks, jobID)
	}
}
