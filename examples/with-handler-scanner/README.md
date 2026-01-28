# With Handler Scanner Example

This example demonstrates the `WithHandlerScanPaths` option for accurately locating method-style gin handlers.

## Features

- **Method-style handlers**: Uses `func (s *APIServer) UpdateContactPermissions(c *gin.Context)` style handlers
- **Handler scanner**: Uses AST scanning to get accurate file/line info instead of `runtime.FuncForPC`
- **Contact API**: A simple CRUD API for contacts with permissions

## Usage

```bash
cd examples/with-handler-scanner
go mod tidy
go run .
```

The generated `openapi.generated.yaml` will have correct file and line references for all handlers.

## Key Code

```go
gen := astra.New(
    inputs.WithGinInput(r),
    outputs.WithOpenAPIOutput("openapi.generated.yaml"),
    // Scan current package for handler locations
    astra.WithHandlerScanPaths(".", "./..."),
)
```

## API Endpoints

| Method | Path | Handler |
|--------|------|---------|
| GET | /contacts | GetContacts |
| GET | /contacts/:id | GetContact |
| POST | /contacts | CreateContact |
| PUT | /contacts/:id | UpdateContact |
| DELETE | /contacts/:id | DeleteContact |
| GET | /contacts/:id/permissions | GetContactPermissions |
| PUT | /contacts/:id/permissions | UpdateContactPermissions |
