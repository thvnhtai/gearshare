package user

type AuthResponse struct {
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

type UserResponse struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
}

func toAuthResponse(r *AuthResult) AuthResponse {
	return AuthResponse{
		AccessToken: r.AccessToken,
		User: UserResponse{
			ID:          r.User.ID,
			Email:       r.User.Email,
			DisplayName: r.User.DisplayName,
			Role:        r.User.Role,
		},
	}
}
