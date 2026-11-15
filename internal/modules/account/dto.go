package account

type deleteAccountRequest struct {
	Password     string `json:"password" binding:"required,max=1024"`
	Confirmation string `json:"confirmation" binding:"required,eq=DELETE"`
}
