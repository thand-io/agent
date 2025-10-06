package models

type ErrorResponse struct {
	Code    int    `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
	// Details string `json:"details"`
}
