package web

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
)

func (s *Server) handleProductHistory(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		http.Error(w, "Kho sản phẩm chưa được khởi tạo", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Mã sản phẩm không hợp lệ", http.StatusBadRequest)
		return
	}
	history, err := s.Products.History(id, 20)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Không đọc được lịch sử sản phẩm", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["products"].ExecuteTemplate(w, "product-history", history); err != nil {
		log.Printf("web: render product history: %v", err)
		http.Error(w, "Không hiển thị được lịch sử sản phẩm", http.StatusInternalServerError)
	}
}
