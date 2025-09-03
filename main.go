package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultBaseURL = "http://192.168.2.207"
	httpTimeout    = 5 * time.Second
	pollInterval   = 2 * time.Second
)

type volumioClient struct {
	baseURL string
	http    *http.Client
}

func newVolumioClient(base string) (*volumioClient, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		return nil, errors.New("URL must include a host")
	}
	return &volumioClient{
		baseURL: u.String(),
		http: &http.Client{
			Timeout: httpTimeout,
		},
	}, nil
}

func (c *volumioClient) cmd(ctx context.Context, command string) error {
	reqURL := fmt.Sprintf("%s/api/v1/commands/?cmd=%s", strings.TrimRight(c.baseURL, "/"), url.QueryEscape(command))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Volumio may respond 200 or 204 for commands; treat 2xx as success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("command %q failed: status %d", command, resp.StatusCode)
	}
	return nil
}

func (c *volumioClient) Play(ctx context.Context) error   { return c.cmd(ctx, "play") }
func (c *volumioClient) Pause(ctx context.Context) error  { return c.cmd(ctx, "pause") }
func (c *volumioClient) Stop(ctx context.Context) error   { return c.cmd(ctx, "stop") }
func (c *volumioClient) Toggle(ctx context.Context) error { return c.cmd(ctx, "toggle") }

// SetVolume sets the absolute volume (0..100).
func (c *volumioClient) SetVolume(ctx context.Context, vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	// Build the query properly so &volume is not escaped into the cmd value.
	reqURL := fmt.Sprintf("%s/api/v1/commands/?cmd=volume&volume=%d", strings.TrimRight(c.baseURL, "/"), vol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("set volume failed: status %d", resp.StatusCode)
	}
	return nil
}

type state struct {
	Status       string  `json:"status"` // "play","pause","stop"
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album"`
	Seek         int64   `json:"seek"`
	Duration     float64 `json:"duration"`
	Volume       int     `json:"volume"`
	Repeat       bool    `json:"repeat"`
	Random       bool    `json:"random"`
	Consume      bool    `json:"consume"`
	VolumioVer   string  `json:"volumio_version"`
	Service      string  `json:"service"`
	TrackType    string  `json:"trackType"`
	Samplerate   string  `json:"samplerate"`
	Bitdepth     string  `json:"bitdepth"`
	Channels     int     `json:"channels"`
	Updated      string  `json:"updated"`
	DisableState bool    `json:"disableUiControls"`
}

func (c *volumioClient) GetState(ctx context.Context) (state, error) {
	var s state
	reqURL := strings.TrimRight(c.baseURL, "/") + "/api/v1/getState"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return s, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s, fmt.Errorf("getState failed: status %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&s); err != nil {
		return s, err
	}
	return s, nil
}

// TUI

type keymap struct {
	PlayPause key.Binding
	Play      key.Binding
	Pause     key.Binding
	Stop      key.Binding
	Refresh   key.Binding
	EditHost  key.Binding
	SaveHost  key.Binding
	Cancel    key.Binding
	Quit      key.Binding
	Help      key.Binding
	VolUp     key.Binding
	VolDown   key.Binding
}

func defaultKeymap() keymap {
	return keymap{
		// Use " " (single space) for the space key
		PlayPause: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle play/pause")),
		Play:      key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play")),
		Pause:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "pause")),
		Stop:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh state")),
		EditHost:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit host URL")),
		SaveHost:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save host")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel edit")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle help")),
		VolUp:     key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "volume up")),
		VolDown:   key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "volume down")),
	}
}

type model struct {
	client     *volumioClient
	hostInput  textinput.Model
	host       string
	st         state
	err        error
	loading    bool
	pollTicker *time.Ticker
	keys       keymap
	help       help.Model
	showHelp   bool
	editing    bool
	connected  bool
}

func initialModel() *model {
	ti := textinput.New()
	ti.Prompt = "Host: "
	ti.SetValue(getDefaultHost())
	ti.CharLimit = 256
	ti.Placeholder = defaultBaseURL
	ti.Blur()

	return &model{
		hostInput: ti,
		host:      ti.Value(),
		keys:      defaultKeymap(),
		help:      help.New(),
	}
}

func getDefaultHost() string {
	if v := strings.TrimSpace(os.Getenv("VOLUMIO_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("VOLUMIO_HOST")); v != "" {
		// Accept host/ip and normalize
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		if !strings.Contains(v, ":") {
			v = v + ":3000"
		}
		return v
	}
	return defaultBaseURL
}

type (
	refreshMsg   state
	errorMsg     error
	connectedMsg struct{ ok bool }
)

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.connectCmd(m.host),
		m.startPolling(),
	)
}

func (m *model) connectCmd(host string) tea.Cmd {
	return func() tea.Msg {
		client, err := newVolumioClient(host)
		if err != nil {
			return errorMsg(err)
		}
		// Quick connectivity probe (resolve host) to provide immediate feedback.
		if err := probeHost(client.baseURL); err != nil {
			m.connected = false
			m.client = client // still set, user can retry
			return errorMsg(fmt.Errorf("connect: %w", err))
		}
		m.client = client
		m.connected = true
		return connectedMsg{ok: true}
	}
}

func probeHost(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		// Try common Volumio port if user omitted it
		host3000 := u.Hostname() + ":3000"
		if c2, err2 := net.DialTimeout("tcp", host3000, 2*time.Second); err2 == nil {
			c2.Close()
			return nil
		}
		return err
	}
	conn.Close()
	return nil
}

func (m *model) startPolling() tea.Cmd {
	return func() tea.Msg {
		m.pollTicker = time.NewTicker(pollInterval)
		return nil
	}
}

func (m *model) refreshCmd() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		st, err := m.client.GetState(ctx)
		if err != nil {
			return errorMsg(err)
		}
		return refreshMsg(st)
	}
}

func (m *model) playCmd() tea.Cmd {
	return m.simpleCmd(func(ctx context.Context) error { return m.client.Play(ctx) })
}

func (m *model) pauseCmd() tea.Cmd {
	return m.simpleCmd(func(ctx context.Context) error { return m.client.Pause(ctx) })
}

func (m *model) stopCmd() tea.Cmd {
	return m.simpleCmd(func(ctx context.Context) error { return m.client.Stop(ctx) })
}

func (m *model) toggleCmd() tea.Cmd {
	return m.simpleCmd(func(ctx context.Context) error { return m.client.Toggle(ctx) })
}

// changeVolume adjusts volume relative to current state by delta and sets it.
func (m *model) changeVolume(delta int) tea.Cmd {
	if m.client == nil {
		return nil
	}
	newVol := m.st.Volume + delta
	if newVol < 0 {
		newVol = 0
	}
	if newVol > 100 {
		newVol = 100
	}
	return m.simpleCmd(func(ctx context.Context) error { return m.client.SetVolume(ctx, newVol) })
}

func (m *model) simpleCmd(fn func(context.Context) error) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			return errorMsg(err)
		}
		// Immediately refresh, then schedule a short delayed refresh
		// to capture any lagging state updates from the backend.
		return tea.Batch(
			m.refreshCmd(),
			tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return m.refreshCmd()() }),
		)()
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.editing {
			switch {
			case key.Matches(msg, m.keys.SaveHost):
				val := strings.TrimSpace(m.hostInput.Value())
				if val == "" {
					return m, nil
				}
				m.host = val
				m.editing = false
				m.hostInput.Blur()
				return m, tea.Batch(
					m.connectCmd(m.host),
					m.refreshCmd(),
				)
			case key.Matches(msg, m.keys.Cancel):
				m.editing = false
				m.hostInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.hostInput, cmd = m.hostInput.Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			if m.pollTicker != nil {
				m.pollTicker.Stop()
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, m.keys.EditHost):
			m.editing = true
			m.hostInput.Focus()
			return m, nil
		case key.Matches(msg, m.keys.PlayPause):
			m.loading = true
			return m, m.toggleCmd()
		case key.Matches(msg, m.keys.Play):
			m.loading = true
			return m, m.playCmd()
		case key.Matches(msg, m.keys.Pause):
			m.loading = true
			return m, m.pauseCmd()
		case key.Matches(msg, m.keys.Stop):
			m.loading = true
			return m, m.stopCmd()
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, m.refreshCmd()
		case key.Matches(msg, m.keys.VolUp):
			m.loading = true
			// Step by 5
			return m, m.changeVolume(5)
		case key.Matches(msg, m.keys.VolDown):
			m.loading = true
			// Step by -5
			return m, m.changeVolume(-5)
		case msg.Type == tea.KeySpace: // fallback for terminals not matching the binding
			m.loading = true
			return m, m.toggleCmd()
		case msg.Type == tea.KeyUp: // fallback for terminals not matching "up"
			m.loading = true
			return m, m.changeVolume(5)
		case msg.Type == tea.KeyDown: // fallback for terminals not matching "down"
			m.loading = true
			return m, m.changeVolume(-5)
		}

	case tea.WindowSizeMsg:
		// no-op for now

	case refreshMsg:
		m.st = state(msg)
		m.err = nil
		m.loading = false
		return m, nil

	case connectedMsg:
		// After a successful connection, perform an initial refresh.
		m.loading = true
		return m, m.refreshCmd()

	case errorMsg:
		m.err = msg
		m.loading = false
		return m, nil

	case tea.Msg:
		// fallthrough
	}

	// Poll ticker
	if m.pollTicker != nil {
		select {
		case <-m.pollTicker.C:
			return m, m.refreshCmd()
		default:
		}
	}

	return m, nil
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	valueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusPlay   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	statusPause  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	statusStop   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	connectedOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	connectedOff = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
)

func (m *model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Volumio TUI Controller"))
	b.WriteString("\n")

	// Connection
	conn := connectedOff.Render("disconnected")
	if m.connected {
		conn = connectedOn.Render("connected")
	}
	b.WriteString(fmt.Sprintf("%s %s  %s %s\n",
		labelStyle.Render("Status:"), conn,
		labelStyle.Render("Host:"), valueStyle.Render(m.host),
	))

	// Edit host
	if m.editing {
		b.WriteString("\n" + m.hostInput.View() + "\n")
		b.WriteString(dimStyle.Render("Press Enter to save, Esc to cancel\n"))
	}
	// Playback info
	statusText := strings.ToLower(m.st.Status)
	switch statusText {
	case "play":
		statusText = statusPlay.Render("PLAY")
	case "pause":
		statusText = statusPause.Render("PAUSE")
	case "stop":
		statusText = statusStop.Render("STOP")
	default:
		statusText = dimStyle.Render(strings.ToUpper(statusText))
	}
	track := "-"
	if m.st.Title != "" {
		parts := []string{m.st.Title}
		if m.st.Artist != "" {
			parts = append(parts, "— "+m.st.Artist)
		}
		if m.st.Album != "" {
			parts = append(parts, "("+m.st.Album+")")
		}
		track = strings.Join(parts, " ")
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Playback:"), statusText))
	b.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Track:   "), valueStyle.Render(track)))
	b.WriteString(fmt.Sprintf("%s %s%%\n", labelStyle.Render("Volume:  "), valueStyle.Render(strconv.Itoa(m.st.Volume))))

	// Error
	if m.err != nil {
		b.WriteString("\n" + errorStyle.Render("Error: "+m.err.Error()) + "\n")
	}

	// Loading
	if m.loading {
		b.WriteString(dimStyle.Render("\nWorking...\n"))
	}

	// Help
	b.WriteString("\n")
	if m.showHelp {
		b.WriteString(m.help.FullHelpView([][]key.Binding{
			{m.keys.PlayPause, m.keys.Play, m.keys.Pause, m.keys.Stop, m.keys.Refresh},
			{m.keys.VolUp, m.keys.VolDown},
			{m.keys.EditHost, m.keys.SaveHost, m.keys.Cancel, m.keys.Help, m.keys.Quit},
		}))
	} else {
		b.WriteString(m.help.ShortHelpView([]key.Binding{
			m.keys.PlayPause, m.keys.Stop, m.keys.VolUp, m.keys.VolDown, m.keys.EditHost, m.keys.Refresh, m.keys.Help, m.keys.Quit,
		}))
	}

	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
