package insight

type generateRequest struct {
	Period    string  `json:"period" binding:"required,oneof=weekly monthly custom"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Focus     *string `json:"focus" binding:"omitempty,max=500"`
}

type feedbackRequest struct {
	Action string  `json:"action" binding:"required,oneof=confirm correct reject"`
	Note   *string `json:"note"`
}
