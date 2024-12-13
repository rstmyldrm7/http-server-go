package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221:", err)
		os.Exit(1)
	}
	defer l.Close()

	fmt.Println("TCP server is running on 0.0.0.0:4221...")

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading request:", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	fmt.Println("Incoming Request:", strings.TrimSpace(requestLine))
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	method, path := parts[0], parts[1]

	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading headers:", err)
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		headerParts := strings.SplitN(line, ":", 2)
		if len(headerParts) == 2 {
			headers[strings.TrimSpace(headerParts[0])] = strings.TrimSpace(headerParts[1])
		}
	}

	if method == "GET" {
		handleGetRequest(conn, path, headers)
	} else if method == "POST" {
		handlePostRequest(conn, path, headers, reader)
	} else {
		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n"))
	}
}

func handleGetRequest(conn net.Conn, path string, headers map[string]string) {
	if path == "/" {
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\nHello, World!\n"))
	} else if path == "/user-agent" {
		userAgent := headers["User-Agent"]
		if userAgent == "" {
			userAgent = "Unknown"
		}
		body := fmt.Sprintf(userAgent)
		headers := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n", len(body))
		conn.Write([]byte(headers + body))
	} else if strings.HasPrefix(path, "/echo/") {
		echoStr := strings.TrimPrefix(path, "/echo/")
		var body []byte
		var headerResp string
		if val, ok := headers["Accept-Encoding"]; ok {
			encodings := strings.Split(val, ",")
			isGzip := false
			for _, encoding := range encodings {
				if strings.TrimSpace(encoding) == "gzip" {
					isGzip = true
					break
				}
			}

			if isGzip {
				var compressedBody bytes.Buffer
				gzipWriter := gzip.NewWriter(&compressedBody)
				_, err := gzipWriter.Write([]byte(echoStr))
				if err != nil {
					conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
					return
				}
				gzipWriter.Close()
				body = compressedBody.Bytes()

				headerResp = fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", len(body))
			} else {
				headerResp = fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n")
			}
		} else {
			body = []byte(echoStr)
			headerResp = fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n", len(echoStr))
		}

		conn.Write([]byte(headerResp))
		conn.Write(body)
	} else if strings.HasPrefix(path, "/files/") {
		fileName := strings.TrimPrefix(path, "/files/")
		dir := os.Args[2]
		file, err := os.Open(fmt.Sprintf("%v%v", dir, fileName))
		if err != nil {
			fmt.Println("Error opening file: ", err.Error())
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
			return
		}
		fileInfo, err := file.Stat()
		if err != nil {
			fmt.Println("Error getting file info: ", err.Error())
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
			return
		}
		fileSize := fileInfo.Size()
		fileContent := make([]byte, fileSize)
		_, err = file.Read(fileContent)
		if err != nil {
			fmt.Println("Error reading file: ", err.Error())
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
			return
		}
		responseStr := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s", fileSize, fileContent)
		conn.Write([]byte(responseStr))
	} else {
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\nPath not found\n"))
	}
}

func handlePostRequest(conn net.Conn, path string, headers map[string]string, reader *bufio.Reader) {
	if strings.HasPrefix(path, "/files") {
		fileName := strings.TrimPrefix(path, "/files/")
		dir := os.Args[2]
		filePath := fmt.Sprintf("%v/%v", dir, fileName)

		contentLengthStr, exists := headers["Content-Length"]
		if !exists {
			conn.Write([]byte("HTTP/1.1 411 Length Required\r\n\r\n"))
			return
		}
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err != nil {
			fmt.Println("Error parsing Content-Length:", err.Error())
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}

		body := make([]byte, contentLength)
		_, err = reader.Read(body)
		if err != nil {
			fmt.Println("Error reading request body:", err.Error())
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}

		file, err := os.Create(filePath)
		if err != nil {
			fmt.Println("Error creating file:", err.Error())
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}
		defer file.Close()

		_, err = file.Write(body)
		if err != nil {
			fmt.Println("Error writing to file:", err.Error())
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		responseStr := fmt.Sprintf("HTTP/1.1 201 Created\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
		conn.Write([]byte(responseStr))
	} else {
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\nPath not found\n"))
	}
}
