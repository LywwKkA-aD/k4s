# k4s demo tape — recorded with VHS (https://github.com/charmbracelet/vhs)
#
# Run from repo root:
#   make demo                                  # ensure k3s + seed are up
#   export KUBECONFIG=$(pwd)/.kube/config
#   vhs docs/demo.tape                         # writes docs/demo.gif
#
# Walks through every top-level feature in order so a viewer can read the
# title row at the top and the kubectl-equivalent footer at the bottom:
#
#   help → contexts → services → namespaces (filter)
#     → pods (filter, describe, logs with search/follow/compact/clear)
#     → deployments (multi-pod logs) → top → watch → home → quit.

Output docs/demo.gif

Set Shell             "zsh"
Set FontSize          14
Set Width             1280
Set Height            720
Set Theme             "Catppuccin Mocha"
Set TypingSpeed       60ms
Set PlaybackSpeed     1.0
Set LoopOffset        0%

Hide
Type "export KUBECONFIG=$(pwd)/.kube/config" Enter
Type "clear" Enter
Show

# === 1. Launch — dashboard with live cluster stats =========================
Type "./bin/k4s"
Sleep 500ms
Enter
Sleep 3s

# === 2. Help popup — show every binding upfront ============================
Type "?"
Sleep 5s
Type "?"
Sleep 1.2s

# === 3. Contexts — kubeconfig context switcher =============================
Type ":"
Sleep 600ms
Type "ctx"
Sleep 1s
Enter
Sleep 4s
Escape
Sleep 1s

# === 4. Services — list + describe =========================================
Type ":"
Sleep 500ms
Type "svc"
Sleep 1s
Enter
Sleep 3s
Down
Sleep 600ms
Enter
Sleep 4s
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 600ms
Escape
Sleep 1s

# === 5. Namespaces — pick the demo namespace via filter ====================
Type ":"
Sleep 500ms
Type "ns"
Sleep 1s
Enter
Sleep 3s
Type "/"
Sleep 800ms
Type "demo"
Sleep 2s
Enter
Sleep 1s
# Synthetic <all> row sticks to the top regardless of the filter, step down.
Down
Sleep 800ms
Enter
Sleep 3s

# === 6. Pods — namespace filter + live "/" filter =========================
# Now in pods, namespace = k4s-demo (header reflects it).
Type "/"
Sleep 800ms
Type "nginx"
Sleep 2.5s
Escape
Sleep 1.2s

# === 7. Describe pod (nginx) ==============================================
# After clearing /nginx the cursor sits on the first row (heartbeat-...).
# Walk down to the first nginx pod and open describe.
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 600ms
Enter
Sleep 4s
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 800ms
Escape
Sleep 1.2s

# === 8. Logs view — narrow to log-spammer, then stream ====================
Type "/"
Sleep 600ms
Type "log"
Sleep 2s
# Commit the filter so only log-spammer pods remain visible.
Enter
Sleep 1s
Type "l"
Sleep 1.2s
# Tail prompt — accept the default tail value.
Enter
Sleep 4s

# Search inside logs for "GET" (nginx-style logs, but log-spammer prints
# its own pattern — search just needs *something* to highlight).
Type "/"
Sleep 600ms
Type "log"
Sleep 1.2s
Enter
Sleep 2.5s
Type "n"
Sleep 1s
Type "n"
Sleep 1s
Type "N"
Sleep 1s
Escape
Sleep 1s

# Toggle full pod names vs compact tag.
Type "t"
Sleep 2.5s
Type "t"
Sleep 2s

# Toggle auto-follow.
Type "f"
Sleep 2.5s
Type "f"
Sleep 2s

# Two-step clear (cancel search → clear buffer).
Type "c"
Sleep 1.2s
Type "c"
Sleep 1.5s

Escape
Sleep 1s

# === 9. Deployments — multi-pod log streaming via 'l' =====================
Type ":"
Sleep 500ms
Type "deploy"
Sleep 1s
Enter
Sleep 3s
Down
Sleep 600ms
Type "l"
Sleep 1s
Enter
Sleep 4s
Escape
Sleep 1.2s

# === 10. Top — kubectl top pods/nodes (graceful "no metrics" hint) ========
Type ":"
Sleep 500ms
Type "top"
Sleep 1s
Enter
Sleep 4s
# Toggle pods / nodes view.
Type "n"
Sleep 2.5s
Type "n"
Sleep 1.5s

# === 11. Watch toggle — visible in the footer hint ========================
Type "w"
Sleep 2.5s
Type "w"
Sleep 1.5s

# === 12. Back home + quit =================================================
Type ":"
Sleep 500ms
Type "home"
Sleep 800ms
Enter
Sleep 2.5s
Type "q"
Sleep 1s
Type "q"
Sleep 1.2s
