package studio

import "errors"

// ErrStoryImageReferencesUnavailable marks story requests blocked before a
// job is created because no configured image provider accepts reference photos.
var ErrStoryImageReferencesUnavailable = errors.New("kể chuyện nhiều cảnh cần ảnh tham chiếu để giữ nhân vật đồng nhất; hiện không có provider miễn phí hỗ trợ. Job chưa được tạo")

// CanCreateFreshStoryJobs reports whether a new story can pass the current
// image-provider capability gate without consuming a topic or calling an LLM.
func (s *Studio) CanCreateFreshStoryJobs() bool {
	if s == nil {
		return false
	}
	return s.mg == nil || supportsImageReferences(s.mg)
}
