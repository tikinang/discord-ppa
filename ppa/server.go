package ppa

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

type sourceInfo struct {
	Name        string
	Description string
}

type server struct {
	s3      *S3Client
	signer  *GPGSigner
	sources []sourceInfo
}

func newServer(s3 *S3Client, signer *GPGSigner, sources []sourceInfo) *server {
	return &server{s3: s3, signer: signer, sources: sources}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /key.gpg", s.handleKeyGPG)
	mux.HandleFunc("GET /dists/", s.handleProxy)
	mux.HandleFunc("GET /pool/", s.handleProxy)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	return mux
}

func (s *server) handleKeyGPG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/pgp-keys")
	w.Write(s.signer.PublicKey())
}

func (s *server) handleProxy(w http.ResponseWriter, r *http.Request) {
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

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, s.indexHTML())
}

func (s *server) indexHTML() string {
	var packageList strings.Builder
	for _, src := range s.sources {
		fmt.Fprintf(&packageList, "<dt><code>%s</code></dt>\n<dd>%s</dd>\n",
			html.EscapeString(src.Name), html.EscapeString(src.Description))
	}

	return `<!DOCTYPE html>
<html>
<head><title>PPA</title></head>
<body>
<h1>PPA</h1>
<p>Unofficial APT repository.</p>
<h2>Available packages</h2>
<dl>
` + packageList.String() + `</dl>
<h2>Setup</h2>
<pre>
# Download the signing key
curl -fsSL https://ppa.matejpavlicek.cz/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/ppa.gpg

# Add the repository
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/ppa.gpg] https://ppa.matejpavlicek.cz stable main" | sudo tee /etc/apt/sources.list.d/ppa.list

# Update and install
sudo apt update
sudo apt install &lt;package-name&gt;
</pre>
</body>
</html>
`
}
