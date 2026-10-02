package web

// helpers.go — tiny cross-domain HTTP helpers shared by every page
// handler: the uniform 303 redirect and the uniform error responder.
// Kept in one place so redirect/error behavior stays identical everywhere.

import (
	"log"
	"net/http"
)

func seeOther(w http.ResponseWriter, r *http.Request, loc string) {
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

func (s *Server) fail(w http.ResponseWriter, err error, what string) {
	log.Printf("web: %s: %v", what, err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
