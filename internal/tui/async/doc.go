// Package async is the asynchronous core the k4s views share: a Token that
// fences late replies, Run for one bounded call, an InFlight set of row
// actions, and a Poller that owns the refresh cadence.
//
// Every reply is a Result carrying the Token its request was started
// under, and a view takes it only while that Token is current. A describe
// meant for pod A never lands in the view for pod B, a tick armed by the
// pods view is dropped by the deployments view, and a second 'e' while the
// first resolves emits nothing.
//
// A list view keeps one Token as its mint, a Poller for the list and an
// InFlight set for its row actions:
//
//	func New(c *k8s.Client, ns string) Model {
//		tok := async.New()
//		poll := async.NewPoller(tok, async.Config{Enabled: c != nil})
//		if c != nil {
//			poll, _ = poll.Begin() // the first load; Init sends it
//		}
//		return Model{client: c, ns: ns, tok: tok, poll: poll}
//	}
//
//	func (m Model) Init() tea.Cmd {
//		if !m.poll.Busy() {
//			return nil
//		}
//		return m.list()
//	}
//
//	func (m Model) list() tea.Cmd {
//		c, ns := m.client, m.ns
//		return async.Run(m.poll.Token(), m.poll.Timeout(),
//			func(ctx context.Context) ([]k8s.Pod, error) { return c.ListPods(ctx, ns) })
//	}
//
//	func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
//		var cmd tea.Cmd
//		var ok bool
//		switch msg := msg.(type) {
//		case async.TickMsg:
//			if m.poll, ok = m.poll.Due(msg); ok {
//				cmd = m.list()
//			}
//		case async.Result[[]k8s.Pod]:
//			if !m.poll.Accept(msg.Token) {
//				return m, nil // another view's reply, or one from before Restart
//			}
//			m.poll, cmd = m.poll.Done(msg.Token, msg.Err)
//			m.err = msg.Err
//			if msg.Err == nil {
//				m.pods = msg.Value
//			}
//		case tea.KeyPressMsg:
//			switch msg.String() {
//			case "r":
//				if m.poll, ok = m.poll.Begin(); ok {
//					cmd = m.list()
//				}
//			case "w":
//				m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled() && m.client != nil)
//			case "e":
//				m.tok = m.tok.Next()
//				if m.acts, ok = m.acts.Begin("exec", m.tok); ok {
//					cmd = async.Run(m.tok, async.DefaultTimeout, m.containersOf(m.selected()))
//				}
//			}
//		}
//		return m, cmd
//	}
//
// The resolve reply is taken with m.acts.End("exec", msg.Token), which is
// false for a stale one. The key hints show m.poll.Hint() ("watch 5s",
// "watch 20s (slow API)", "paused"), and the footer notes m.poll.Status()
// while backed off. When the namespace or mode changes in place, the view
// moves its poller to a new epoch so the old replies are dropped:
//
//	m.tok = m.tok.Next()
//	m.poll = m.poll.Restart(m.tok)
//	return m, m.list()
//
// A view with two pollers gives each its own Token from the mint.
package async
