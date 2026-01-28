package types

// Contact represents a user contact.
type Contact struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone,omitempty"`
	CreatedAt string `json:"created_at"`
}

// CreateContactRequest is the request body for creating a contact.
type CreateContactRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
	Phone string `json:"phone,omitempty"`
}

// UpdateContactRequest is the request body for updating a contact.
type UpdateContactRequest struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// ContactPermissions represents the permissions for a contact.
type ContactPermissions struct {
	ContactID int  `json:"contact_id"`
	CanView   bool `json:"can_view"`
	CanEdit   bool `json:"can_edit"`
	CanDelete bool `json:"can_delete"`
}

// UpdatePermissionsRequest is the request body for updating contact permissions.
type UpdatePermissionsRequest struct {
	CanView   bool `json:"can_view"`
	CanEdit   bool `json:"can_edit"`
	CanDelete bool `json:"can_delete"`
}

// FriendRequestStatus represents the status of a friend request.
type FriendRequestStatus uint8

const (
	FriendRequestStatusPending  FriendRequestStatus = 1
	FriendRequestStatusAccepted FriendRequestStatus = 2
	FriendRequestStatusExpired  FriendRequestStatus = 3
)

// FriendRequest represents a friend request.
type FriendRequest struct {
	ID        int                 `json:"id"`
	FromUser  string              `json:"from_user"`
	ToUser    string              `json:"to_user"`
	Status    FriendRequestStatus `json:"status"`
	Message   string              `json:"message,omitempty"`
	CreatedAt string              `json:"created_at"`
}
