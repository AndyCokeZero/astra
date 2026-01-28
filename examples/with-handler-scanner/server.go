package main

import (
	"net/http"
	"strconv"
	"time"
	"with-handler-scanner/types"

	"github.com/gin-gonic/gin"
)

// APIServer is the main API server struct that holds all handlers.
type APIServer struct {
	contacts       map[int]types.Contact
	friendRequests map[int]types.FriendRequest
	nextID         int
}

// NewAPIServer creates a new API server instance.
func NewAPIServer() *APIServer {
	return &APIServer{
		contacts:       make(map[int]types.Contact),
		friendRequests: make(map[int]types.FriendRequest),
		nextID:         1,
	}
}

// GetContacts returns all contacts.
func (s *APIServer) GetContacts(c *gin.Context) {
	contacts := make([]types.Contact, 0, len(s.contacts))
	for _, contact := range s.contacts {
		contacts = append(contacts, contact)
	}
	c.JSON(http.StatusOK, contacts)
}

// GetContact returns a single contact by ID.
func (s *APIServer) GetContact(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	contact, ok := s.contacts[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	c.JSON(http.StatusOK, contact)
}

// CreateContact creates a new contact.
func (s *APIServer) CreateContact(c *gin.Context) {
	var req types.CreateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	contact := types.Contact{
		ID:        s.nextID,
		Name:      req.Name,
		Email:     req.Email,
		Phone:     req.Phone,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.contacts[s.nextID] = contact
	s.nextID++

	c.JSON(http.StatusCreated, contact)
}

// UpdateContact updates an existing contact.
func (s *APIServer) UpdateContact(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	contact, ok := s.contacts[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	var req types.UpdateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		contact.Name = req.Name
	}
	if req.Email != "" {
		contact.Email = req.Email
	}
	if req.Phone != "" {
		contact.Phone = req.Phone
	}
	s.contacts[id] = contact

	c.JSON(http.StatusOK, contact)
}

// DeleteContact deletes a contact by ID.
func (s *APIServer) DeleteContact(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if _, ok := s.contacts[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	delete(s.contacts, id)
	c.JSON(http.StatusNoContent, nil)
}

// GetContactPermissions returns permissions for a contact.
func (s *APIServer) GetContactPermissions(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if _, ok := s.contacts[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	// Return mock permissions
	permissions := types.ContactPermissions{
		ContactID: id,
		CanView:   true,
		CanEdit:   true,
		CanDelete: false,
	}
	c.JSON(http.StatusOK, permissions)
}

// UpdateContactPermissions updates permissions for a contact.
func (s *APIServer) UpdateContactPermissions(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if _, ok := s.contacts[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "contact not found"})
		return
	}

	var req types.UpdatePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Return updated permissions
	permissions := types.ContactPermissions{
		ContactID: id,
		CanView:   req.CanView,
		CanEdit:   req.CanEdit,
		CanDelete: req.CanDelete,
	}
	c.JSON(http.StatusOK, permissions)
}

// GetFriendRequests returns all friend requests.
func (s *APIServer) GetFriendRequests(c *gin.Context) {
	requests := make([]types.FriendRequest, 0, len(s.friendRequests))
	for _, req := range s.friendRequests {
		requests = append(requests, req)
	}
	c.JSON(http.StatusOK, requests)
}

// GetFriendRequest returns a single friend request by ID.
func (s *APIServer) GetFriendRequest(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	req, ok := s.friendRequests[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "friend request not found"})
		return
	}

	c.JSON(http.StatusOK, req)
}

// CreateFriendRequest creates a new friend request.
func (s *APIServer) CreateFriendRequest(c *gin.Context) {
	var req struct {
		ToUser  string `json:"to_user" binding:"required"`
		Message string `json:"message,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	friendReq := types.FriendRequest{
		ID:        s.nextID,
		FromUser:  "current_user", // Simulated
		ToUser:    req.ToUser,
		Status:    types.FriendRequestStatusPending,
		Message:   req.Message,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.friendRequests[s.nextID] = friendReq
	s.nextID++

	c.JSON(http.StatusCreated, friendReq)
}

// UpdateFriendRequestStatus updates the status of a friend request.
func (s *APIServer) UpdateFriendRequestStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	friendReq, ok := s.friendRequests[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "friend request not found"})
		return
	}

	var req struct {
		Status types.FriendRequestStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	friendReq.Status = req.Status
	s.friendRequests[id] = friendReq

	c.JSON(http.StatusOK, friendReq)
}
