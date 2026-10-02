import http.server
import socketserver
import os
import glob
from urllib.parse import parse_qs
from datetime import datetime

PORT = 4815
NOTES_DIR = "/opt/data/project-hail-mary/notes"
os.makedirs(NOTES_DIR, exist_ok=True)

class HailMaryHandler(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length).decode('utf-8')
        params = parse_qs(post_data)
        
        content = params.get('content', [''])[0].strip()
        if content:
            timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
            filename = f"note-{timestamp}.md"
            filepath = os.path.join(NOTES_DIR, filename)
            with open(filepath, 'w', encoding='utf-8') as f:
                f.write(content)
        
        # Redirect back to homepage
        self.send_response(303)
        self.send_header('Location', '/')
        self.end_headers()

    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Type', 'text/html; charset=utf-8')
        self.end_headers()

        # Scan all .md files in NOTES_DIR
        notes = []
        for file_path in sorted(glob.glob(os.path.join(NOTES_DIR, "*.md")), reverse=True):
            filename = os.path.basename(file_path)
            with open(file_path, 'r', encoding='utf-8') as f:
                lines = f.readlines()
                title = filename
                for line in lines:
                    if line.startswith("# "):
                        title = line.replace("# ", "").strip()
                        break
                content = "".join(lines)
                notes.append({"filename": filename, "title": title, "content": content})

        # Render HTML with Tailwind CSS
        cards_html = ""
        for n in notes:
            cards_html += f"""
            <div class="bg-gray-900 border border-gray-800 rounded-xl p-4 shadow space-y-2">
                <div class="text-xs text-gray-500 flex justify-between">
                    <span>📄 {n['filename']}</span>
                </div>
                <div class="text-md font-semibold text-emerald-400">{n['title']}</div>
                <pre class="text-sm text-gray-300 whitespace-pre-wrap font-sans bg-gray-950 p-3 rounded-lg border border-gray-800/50">{n['content']}</pre>
            </div>
            """

        html = f"""<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <title>Project Hail Mary 🚀</title>
            <script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
        </head>
        <body class="bg-gray-950 text-gray-100 min-h-screen p-6 font-sans">
            <div class="max-w-2xl mx-auto space-y-6">
                <header class="border-b border-gray-800 pb-4">
                    <h1 class="text-2xl font-bold tracking-tight text-emerald-400">Project Hail Mary 🚀</h1>
                    <p class="text-sm text-gray-400">Markdown-native personal productivity dashboard</p>
                </header>

                <!-- Quick Capture Box -->
                <form action="/" method="POST" class="bg-gray-900 border border-gray-800 rounded-xl p-4 shadow-lg space-y-3">
                    <textarea name="content" rows="3" placeholder="What's on your mind? (Supports Markdown # headers)..." class="w-full bg-gray-950 border border-gray-800 rounded-lg p-3 text-gray-100 focus:outline-none focus:border-emerald-500 placeholder-gray-600"></textarea>
                    <div class="flex justify-end">
                        <button type="submit" class="bg-emerald-600 hover:bg-emerald-500 text-white px-4 py-2 rounded-lg text-sm font-medium transition cursor-pointer">Quick Save</button>
                    </div>
                </form>

                <!-- Notes Timeline -->
                <div class="space-y-4">
                    <h3 class="text-lg font-semibold text-gray-300">Timeline ({len(notes)} Notes)</h3>
                    {cards_html if cards_html else '<p class="text-sm text-gray-500 italic">No notes found in ./notes</p>'}
                </div>
            </div>
        </body>
        </html>
        """
        self.wfile.write(html.encode('utf-8'))

if __name__ == '__main__':
    print(f"Project Hail Mary running at http://localhost:{PORT}")
    with socketserver.TCPServer(("", PORT), HailMaryHandler) as httpd:
        httpd.serve_forever()
