package web

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
)

func (s *Server) handleConfigJSONImport(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}

	// Hỗ trợ tối đa 10MB upload
	if err := r.ParseMultipartForm(10 << 20); err != nil && err != http.ErrNotMultipart {
		if err := r.ParseForm(); err != nil {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Không đọc được dữ liệu nhập: "+err.Error()))
			return
		}
	}

	var rawBytes []byte
	file, _, err := r.FormFile("json_file")
	if err == nil && file != nil {
		defer file.Close()
		rawBytes, _ = io.ReadAll(file)
	} else {
		rawBytes = []byte(r.FormValue("json_content"))
	}

	rawBytes = bytes.TrimSpace(rawBytes)
	if len(rawBytes) == 0 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa có nội dung hoặc file cấu hình nào được chọn."))
		return
	}

	if rawBytes[0] != '{' {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chỉ hỗ trợ nhập cấu hình JSON."))
		return
	}
	s.importConfigJSON(w, r, rawBytes)
}
