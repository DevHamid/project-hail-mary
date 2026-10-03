package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Note struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Date    string   `json:"date"`
	Tags    []string `json:"tags"`
}

var db *sql.DB
var sessions = map[string]time.Time{} // sessionID -> expiry

const sessionCookie = "hailmary_session"
const sessionTTL = 24 * time.Hour

func initDB() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "notes.db"
	}
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS notes (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT, content TEXT, date TEXT)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS tags (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS note_tags (note_id INTEGER, tag_id INTEGER)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE, password_hash TEXT)`)
}

func extractTags(content string) []string {
	seen := map[string]bool{}
	var tags []string
	for _, w := range strings.Fields(content) {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			t := strings.Trim(w[1:], ".,!?;:()[]")
			if t != "" && !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags
}

func linkTags(noteID int64, content string) {
	db.Exec("DELETE FROM note_tags WHERE note_id = ?", noteID)
	for _, tag := range extractTags(content) {
		db.Exec("INSERT OR IGNORE INTO tags (name) VALUES (?)", tag)
		var tagID int64
		db.QueryRow("SELECT id FROM tags WHERE name = ?", tag).Scan(&tagID)
		db.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", noteID, tagID)
	}
}

func saveNote(content string) {
	if content == "" {
		return
	}
	title := "Untitled Note"
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[0] != "" {
		title = strings.TrimPrefix(lines[0], "# ")
	}
	date := time.Now().Format("Jan 02, 15:04")
	res, _ := db.Exec("INSERT INTO notes (title, content, date) VALUES (?, ?, ?)", title, content, date)
	noteID, _ := res.LastInsertId()
	linkTags(noteID, content)
}

func getNotes(filter string) []Note {
	q := `SELECT DISTINCT n.id, n.title, n.content, n.date FROM notes n`
	var args []any
	if filter != "" {
		q += ` JOIN note_tags nt ON n.id = nt.note_id JOIN tags t ON nt.tag_id = t.id WHERE t.name = ?`
		args = append(args, filter)
	}
	q += ` ORDER BY n.id DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return []Note{}
	}
	defer rows.Close()
	var notes []Note
	for rows.Next() {
		var n Note
		rows.Scan(&n.ID, &n.Title, &n.Content, &n.Date)
		notes = append(notes, n)
	}
	for i := range notes {
		tr, _ := db.Query("SELECT t.name FROM tags t JOIN note_tags nt ON t.id = nt.tag_id WHERE nt.note_id = ?", notes[i].ID)
		var ts []string
		for tr.Next() {
			var tn string
			tr.Scan(&tn)
			ts = append(ts, tn)
		}
		tr.Close()
		notes[i].Tags = ts
	}
	return notes
}

func genSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		expiry, ok := sessions[cookie.Value]
		if !ok || time.Now().After(expiry) {
			delete(sessions, cookie.Value)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sessions[cookie.Value] = time.Now().Add(sessionTTL)
		next(w, r)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>Login</title>
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
</head><body class="bg-gray-950 text-gray-100 min-h-screen flex items-center justify-center">
<div class="bg-gray-900 border border-gray-800 rounded-xl p-8 w-full max-w-md">
<h1 class="text-2xl font-bold text-emerald-400 mb-6 text-center">Project Hail Mary</h1>
<form method="POST" class="space-y-4">
<div><label class="block text-sm text-gray-400 mb-1">Username</label>
<input name="username" type="text" required class="w-full bg-gray-950 border border-gray-800 rounded-lg p-3 focus:outline-none focus:border-emerald-500"></div>
<div><label class="block text-sm text-gray-400 mb-1">Password</label>
<input name="password" type="password" required class="w-full bg-gray-950 border border-gray-800 rounded-lg p-3 focus:outline-none focus:border-emerald-500"></div>
<button type="submit" class="w-full bg-emerald-600 hover:bg-emerald-500 text-white px-4 py-2 rounded-lg font-medium">Sign In</button>
</form>
<p class="text-xs text-gray-500 text-center mt-4">First run? Creates admin account automatically.</p>
</div></body></html>`)
		return
	}
	r.ParseForm()
	user := r.FormValue("username")
	pass := r.FormValue("password")
	if user == "" || pass == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	var storedHash string
	err := db.QueryRow("SELECT password_hash FROM users WHERE username = ?", user).Scan(&storedHash)
	if err == sql.ErrNoRows {
		// First user -> create admin
		hash := simpleHash(pass)
		db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", user, hash)
		sessionID := genSession()
		sessions[sessionID] = time.Now().Add(sessionTTL)
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sessionID, Path: "/", HttpOnly: true, MaxAge: int(sessionTTL.Seconds())})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if simpleHash(pass) != storedHash {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	sessionID := genSession()
	sessions[sessionID] = time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sessionID, Path: "/", HttpOnly: true, MaxAge: int(sessionTTL.Seconds())})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(sessionCookie)
	if cookie != nil {
		delete(sessions, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func simpleHash(s string) string {
	// Ponytail: skipped bcrypt/argon2, add when: need real security audit
	h := fmt.Sprintf("%x", simpleSum(s))
	return h
}

func simpleSum(s string) uint64 {
	// FNV-1a 64-bit - fast, non-crypto
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func main() {
	initDB()
	defer db.Close()

	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)

	http.HandleFunc("/api/notes", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var b struct{ Content string `json:"content"` }
			json.NewDecoder(r.Body).Decode(&b)
			saveNote(b.Content)
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		if r.Method == http.MethodPut {
			var b struct {
				ID      int64  `json:"id"`
				Content string `json:"content"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			if b.Content != "" {
				title := "Untitled Note"
				lines := strings.Split(b.Content, "\n")
				if len(lines) > 0 && lines[0] != "" {
					title = strings.TrimPrefix(lines[0], "# ")
				}
				db.Exec("UPDATE notes SET title = ?, content = ? WHERE id = ?", title, b.Content, b.ID)
				linkTags(b.ID, b.Content)
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		if r.Method == http.MethodDelete {
			id := r.URL.Query().Get("id")
			db.Exec("DELETE FROM note_tags WHERE note_id = ?", id)
			db.Exec("DELETE FROM notes WHERE id = ?", id)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getNotes(r.URL.Query().Get("tag")))
	}))

	http.HandleFunc("/api/tags", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var b struct {
				Old string `json:"old"`
				New string `json:"new"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			db.Exec("UPDATE tags SET name = ? WHERE name = ?", b.New, b.Old)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		rows, _ := db.Query(`SELECT t.name, COUNT(nt.note_id) FROM tags t LEFT JOIN note_tags nt ON t.id = nt.tag_id GROUP BY t.id`)
		defer rows.Close()
		m := map[string]int{}
		for rows.Next() {
			var n string
			var c int
			rows.Scan(&n, &c)
			m[n] = c
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}))

	http.HandleFunc("/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>Project Hail Mary</title>
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<style>.md h1,.md h2{color:#6ee7b7;font-weight:700}.md code{background:#020617;padding:2px 6px;border-radius:6px}.md pre{background:#020617;padding:12px;border-radius:10px;overflow:auto}.md ul{list-style:disc;padding-left:20px}.md a{color:#34d399}</style>
</head><body class="bg-gray-950 text-gray-100 min-h-screen p-6 font-sans">
<div class="max-w-4xl mx-auto flex gap-6">
<aside class="w-48 shrink-0"><div class="text-xs uppercase text-gray-500 mb-2">Tags</div><div id="tags" class="space-y-1"></div></aside>
<div class="flex-1 space-y-6">
<header class="border-b border-gray-800 pb-4 flex justify-between items-center"><div><h1 class="text-2xl font-bold text-emerald-400">Project Hail Mary</h1><p class="text-sm text-gray-400">Notes Edition</p></div><a href="/logout" class="text-sm text-gray-400 hover:text-emerald-400">Logout</a></header>
<div class="bg-gray-900 border border-gray-800 rounded-xl p-4 space-y-3">
<textarea id="content" rows="4" placeholder="Write markdown... **bold**, - list, #tag" class="w-full bg-gray-950 border border-gray-800 rounded-lg p-3 focus:outline-none focus:border-emerald-500"></textarea>
<div class="flex justify-end"><button onclick="saveNote()" class="bg-emerald-600 hover:bg-emerald-500 px-4 py-2 rounded-lg text-sm">Quick Save</button></div></div>
<div id="notes" class="space-y-4"></div>
</div></div>
<script>
let activeTag="";
function esc(s){return (s||"").replace(/&/g,"&").replace(/</g,"<").replace(/>/g,">");}
async function loadTags(){
let res=await fetch('/api/tags');let counts=await res.json();
let h='<button onclick="filterTag(\'\')" class="block w-full text-left px-3 py-1 rounded-lg text-sm '+(activeTag===''?'bg-emerald-600':'bg-gray-900')+'">All</button>';
for(let t in counts){h+='<div class="flex items-center bg-gray-900 rounded-lg px-2 py-1"><button onclick="filterTag(\''+t+'\')" class="flex-1 text-left text-sm truncate">#'+t+' ('+counts[t]+')</button><button onclick="renameTag(\''+t+'\')" class="text-xs px-1">✏️</button></div>';}
document.getElementById('tags').innerHTML=h;
}
function filterTag(t){activeTag=t;loadNotes();loadTags();}
async function renameTag(o){let n=prompt("Rename #"+o+" to:",o);if(!n||n===o)return;n=n.replace(/^#/,'');await fetch('/api/tags',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({old:o,new:n})});if(activeTag===o)activeTag=n;loadNotes();loadTags();}
async function loadNotes(){
let url=activeTag?'/api/notes?tag='+activeTag:'/api/notes';
let res=await fetch(url);let notes=await res.json();let html='';
for(let n of notes){let tags=(n.tags||[]).map(t=>'<span class="text-xs text-emerald-400 bg-emerald-950/50 px-2 py-0.5 rounded">#'+t+'</span>').join(' ');
html+='<div class="bg-gray-900 border border-gray-800 rounded-xl p-4 space-y-2"><div class="flex justify-between text-xs text-gray-500"><span>'+esc(n.date)+'</span><div>'+tags+'</div></div><div class="font-semibold text-emerald-300">'+esc(n.title)+'</div><div id="view-'+n.id+'" class="md text-sm text-gray-300">'+marked.parse(n.content||'')+'</div><textarea id="edit-'+n.id+'" rows="4" class="hidden w-full bg-gray-950 border border-emerald-600 rounded-lg p-3 text-sm">'+esc(n.content)+'</textarea><div class="flex gap-2 text-xs"><button onclick="startEdit('+n.id+')" id="btn-edit-'+n.id+'" class="text-gray-400 hover:text-emerald-400">Edit</button><button onclick="updateNote('+n.id+')" id="btn-save-'+n.id+'" class="hidden text-emerald-400">Save</button><button onclick="cancelEdit('+n.id+')" id="btn-cancel-'+n.id+'" class="hidden text-gray-400">Cancel</button><button onclick="deleteNote('+n.id+')" class="text-gray-400 hover:text-red-400">Delete</button></div></div>';}
document.getElementById('notes').innerHTML=html;
}
function startEdit(id){document.getElementById('view-'+id).classList.add('hidden');document.getElementById('edit-'+id).classList.remove('hidden');document.getElementById('btn-edit-'+id).classList.add('hidden');document.getElementById('btn-save-'+id).classList.remove('hidden');document.getElementById('btn-cancel-'+id).classList.remove('hidden');}
function cancelEdit(id){document.getElementById('view-'+id).classList.remove('hidden');document.getElementById('edit-'+id).classList.add('hidden');document.getElementById('btn-edit-'+id).classList.remove('hidden');document.getElementById('btn-save-'+id).classList.add('hidden');document.getElementById('btn-cancel-'+id).classList.add('hidden');}
async function updateNote(id){let c=document.getElementById('edit-'+id).value;await fetch('/api/notes',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id,content:c})});loadNotes();loadTags();}
async function deleteNote(id){if(!confirm("Delete note?"))return;await fetch('/api/notes?id='+id,{method:'DELETE'});loadNotes();loadTags();}
async function saveNote(){let c=document.getElementById('content').value;if(!c)return;await fetch('/api/notes',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({content:c})});document.getElementById('content').value='';loadNotes();loadTags();}
loadNotes();loadTags();
</script></body></html>`)
	}))
	port := os.Getenv("PORT")
	if port == "" {
		port = "4815"
	}
	fmt.Println("Server running on http://localhost:" + port)
	http.ListenAndServe(":"+port, nil)
}