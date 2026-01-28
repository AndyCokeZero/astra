package main

import (
	"github.com/gin-gonic/gin"
	"github.com/ls6-events/astra"
	"github.com/ls6-events/astra/inputs"
	"github.com/ls6-events/astra/outputs"
)

func main() {
	r := gin.Default()

	// Create API server with method-style handlers
	api := NewAPIServer()

	// Register routes using method handlers
	r.GET("/contacts", api.GetContacts)
	r.GET("/contacts/:id", api.GetContact)
	r.POST("/contacts", api.CreateContact)
	r.PUT("/contacts/:id", api.UpdateContact)
	r.DELETE("/contacts/:id", api.DeleteContact)

	// Permissions routes - uses method handlers like UpdateContactPermissions
	r.GET("/contacts/:id/permissions", api.GetContactPermissions)
	r.PUT("/contacts/:id/permissions", api.UpdateContactPermissions)

	// Friend request routes - demonstrates enum type (FriendRequestStatus)
	r.GET("/friend-requests", api.GetFriendRequests)
	r.GET("/friend-requests/:id", api.GetFriendRequest)
	r.POST("/friend-requests", api.CreateFriendRequest)
	r.PUT("/friend-requests/:id/status", api.UpdateFriendRequestStatus)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Create astra service with handler scanner
	// This demonstrates the new WithHandlerScanPaths option
	gen := astra.New(
		inputs.WithGinInput(r),
		outputs.WithOpenAPIOutput("openapi.generated.yaml"),
		// Scan current package for handler locations
		// This enables accurate file/line info for method-style handlers
		astra.WithHandlerScanPaths(".", "./..."),
	)

	config := astra.Config{
		Title:       "Contact API",
		Description: "Example API demonstrating method-style handlers with handler scanner",
		Version:     "1.0.0",
		Host:        "localhost",
		Port:        8000,
	}

	gen.SetConfig(&config)

	err := gen.Parse()
	if err != nil {
		panic(err)
	}

	err = r.Run(":8000")
	if err != nil {
		panic(err)
	}
}
