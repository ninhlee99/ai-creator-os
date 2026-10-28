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

	// Form parser's maxMemory only controls RAM; cap request and imported JSON bytes too.
	r.Body = http.MaxBytesReader(w, r.Body, (11 << 20))
	parseErr := r.ParseMultipartForm(10 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil && parseErr != http.ErrNotMultipart {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Không đọc được dữ liệu nhập: "+parseErr.Error()))
		return
	}
	if parseErr == http.ErrNotMultipart {
		if err := r.ParseForm(); err != nil {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Không đọc được dữ liệu nhập: "+err.Error()))
			return
		}
	}

	var rawBytes []byte
	file, _, err := r.FormFile("json_file")
	if err == nil && file != nil {
		defer file.Close()
		rawBytes, err = io.ReadAll(io.LimitReader(file, (10<<20)+1))
		if err != nil {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Không đọc được file cấu hình: "+err.Error()))
			return
		}
	} else {
		rawBytes = []byte(r.FormValue("json_content"))
	}
	if len(rawBytes) > 10<<20 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("File cấu hình vượt quá 10MB."))
		return
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
