# Virtual Disk Go Implementation

A Go implementation of a virtual disk system with memory buffering and HTTP API.

## Features

- Memory buffering for read/write operations
- Gradual writing to disk to minimize wear and tear
- Support for S3-like path syntax for file access
- RESTful HTTP API endpoints
- CORS support
- JSON logging

## API Endpoints

- `POST /files/*path` - Write a file
  - Request body: `{"data": "base64_encoded_data"}`
  - Response: `{"success": true/false}`

- `GET /files/*path` - Read a file
  - Response: `{"data": "base64_encoded_data"}`

- `GET /list/*prefix` - List files
  - Response: `{"files": ["file1", "file2", ...]}`

- `DELETE /files/*path` - Delete a file
  - Response: `{"success": true/false}`

## Building and Running

1. Install dependencies:
```bash
go mod tidy
```

2. Build the server:
```bash
go build -o server ./cmd/server
```

3. Run the server:
```bash
./server
```

The server will start on port 3000.

## Example Usage

### Writing a file
```bash
curl -X POST http://localhost:3000/files/example.txt \
  -H "Content-Type: application/json" \
  -d '{"data":"SGVsbG8gV29ybGQh"}'  # "Hello World!" in base64
```

### Reading a file
```bash
curl http://localhost:3000/files/example.txt
```

### Listing files
```bash
curl http://localhost:3000/list/
```

### Deleting a file
```bash
curl -X DELETE http://localhost:3000/files/example.txt
```
