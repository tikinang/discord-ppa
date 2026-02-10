package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Server struct {
	s3     *S3Client
	signer *GPGSigner
}

func NewServer(s3 *S3Client, signer *GPGSigner) *Server {
	return &Server{s3: s3, signer: signer}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /key.gpg", s.handleKeyGPG)
	mux.HandleFunc("GET /dists/", s.handleProxy)
	mux.HandleFunc("GET /pool/", s.handleProxy)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	return mux
}

func (s *Server) handleKeyGPG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/pgp-keys")
	w.Write(s.signer.PublicKey())
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Contains(key, "..") {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	output, err := s.s3.GetObject(r.Context(), key)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	defer output.Body.Close()

	if output.ContentType != nil {
		w.Header().Set("Content-Type", *output.ContentType)
	}
	if output.ContentLength != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", *output.ContentLength))
	}

	io.Copy(w, output.Body)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

const indexHTML = `<!DOCTYPE html>
<html>
<head><title>Discord PPA</title></head>
<body>
<h1>Discord PPA</h1>
<p>Unofficial APT repository for Discord on Linux.</p>
<h2>Setup</h2>
<pre>
# Download the signing key
curl -fsSL https://ppa.matejpavlicek.cz/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/discord-ppa.gpg

# Add the repository
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/discord-ppa.gpg] https://ppa.matejpavlicek.cz stable main" | sudo tee /etc/apt/sources.list.d/discord-ppa.list

# Update and install
sudo apt update
sudo apt install discord
</pre>
</body>
</html>
`
