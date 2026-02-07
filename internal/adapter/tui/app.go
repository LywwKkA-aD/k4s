package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/adapter/k8s"
	"github.com/LywwKkA-aD/k4s/internal/adapter/ssh"
	"github.com/LywwKkA-aD/k4s/internal/domain"
	"github.com/LywwKkA-aD/k4s/internal/logger"
)

const podRefreshInterval = 5 * time.Second

// ViewState represents the current view
type ViewState int

const (
	ViewKubeConfigSelect ViewState = iota
	ViewConnecting
	ViewNamespaces
	ViewPods
	ViewPodDetails
	ViewLogs
	ViewMain
	ViewSSHHosts
	ViewSSHConnecting
	ViewCrictlContainers
	ViewCrictlLogs
	ViewNodeInfo
	ViewDeployments
	ViewDeploymentDetails
	ViewServices
	ViewServiceDetails
	ViewEvents
	ViewMultiPodLogs
)

// Messages for async operations
type connectResultMsg struct {
	client      *k8s.Client
	clusterInfo *domain.ClusterInfo
	err         error
}

type namespacesResultMsg struct {
	namespaces []domain.Namespace
	err        error
}

type podsResultMsg struct {
	pods []domain.Pod
	err  error
}

type podDetailsResultMsg struct {
	pod    *domain.Pod
	events []domain.PodEvent
	err    error
}

type podRefreshTickMsg struct{}
type eventRefreshTickMsg struct{}

type podDeleteResultMsg struct {
	podName string
	err     error
}

type podRestartResultMsg struct {
	podName string
	err     error
}

type logsResultMsg struct {
	logs string
	err  error
}

type logLineMsg struct {
	line string
}

type logStreamEndedMsg struct {
	err error
}

type multiPodLogLineMsg struct {
	podName   string
	container string
	line      string
}

type multiPodLogStreamEndedMsg struct {
	podName string
	err     error
}

type containersResultMsg struct {
	containers []string
	err        error
}

// SSH-related messages
type sshConnectResultMsg struct {
	err error
}

type sshCrictlContainersMsg struct {
	containers []ssh.CrictlContainer
	err        error
}

type sshNodeInfoMsg struct {
	info *domain.NodeInfo
	err  error
}

type sshCrictlLogsMsg struct {
	logs string
	err  error
}

type sshCrictlLogLineMsg struct {
	line string
}

type sshCrictlLogStreamEndedMsg struct {
	err error
}

// Deployment-related messages
type deploymentsResultMsg struct {
	deployments []domain.Deployment
	err         error
}

type deploymentDetailsResultMsg struct {
	deployment *domain.Deployment
	err        error
}

type deploymentScaleResultMsg struct {
	deploymentName string
	replicas       int32
	err            error
}

type deploymentRestartResultMsg struct {
	deploymentName string
	err            error
}

type deploymentDeleteResultMsg struct {
	deploymentName string
	err            error
}

// Service-related messages
type servicesResultMsg struct {
	services []domain.Service
	err      error
}

type serviceDetailsResultMsg struct {
	service *domain.Service
	err     error
}

// Event-related messages
type eventsResultMsg struct {
	events []domain.Event
	err    error
}

// Metrics-related messages
type metricsResultMsg struct {
	metrics map[string]domain.PodMetrics
	err     error
}

// App is the main TUI application model
type App struct {
	styles             Styles
	width              int
	height             int
	contentWidth       int  // width for main content area (excludes sidebar + padding)
	sidebarWidth       int  // sidebar panel width (0 when hidden)
	showSidebar        bool // false when terminal < sidebarMinWidth
	ready              bool
	config             *domain.Config
	selectedConfig     *domain.KubeConfig
	viewState          ViewState
	kubeConfigList     list.Model
	namespaceList      list.Model
	podList            list.Model
	podDetails         PodDetailsModel
	selectedPodName    string
	pods               []domain.Pod
	k8sClient          *k8s.Client
	clusterInfo        *domain.ClusterInfo
	connectionStatus   domain.ConnectionStatus
	spinner            spinner.Model
	err                error
	loading            bool
	podCount           int
	namespaceCount     int
	confirmDialog      ConfirmDialog
	notification       Notification
	logViewer          LogViewer
	logSourceView      ViewState // Track where we came from when viewing logs
	containerSelector  ContainerSelector
	logStreamCancel    context.CancelFunc
	logStreamActive    bool
	logLineChan        <-chan string

	// SSH-related fields
	sshHostList             list.Model
	sshClient               *ssh.Client
	selectedSSHHost         *domain.SSHHost
	crictlContainers        []ssh.CrictlContainer
	crictlContainerList     list.Model
	nodeInfo                *domain.NodeInfo
	passphraseInput         PassphraseInput
	crictlLogViewer         CrictlLogViewer
	selectedCrictlContainer *ssh.CrictlContainer
	crictlLogStreamCancel   context.CancelFunc
	crictlLogStreamActive   bool
	crictlLogLineChan       <-chan string

	// Help screen
	helpScreen HelpScreen

	// Search input
	searchInput SearchInput

	// Deployments view
	deploymentList       list.Model
	deploymentCount      int
	deploymentDetails    DeploymentDetailsModel
	selectedDeployName   string

	// Services view
	serviceList        list.Model
	serviceCount       int
	serviceDetails     ServiceDetailsModel
	selectedServiceName string

	// Events view
	eventViewer EventViewer

	// Metrics
	metricsClient    *k8s.MetricsClient
	metricsAvailable bool
	metricsEnabled   bool
	podMetrics       map[string]domain.PodMetrics

	// Scale dialog
	scaleDialog ScaleDialog

	// Multi-pod log fields
	podMultiSelector       PodMultiSelector
	multiPodLogViewer      MultiPodLogViewer
	multiPodStreamCancel   context.CancelFunc
	multiPodStreamCtx      context.Context
	multiPodStreamActive   bool
	multiPodActiveStreams   int
	multiPodLineChanMap    map[string]<-chan string
	multiPodContainerMap   map[string]string // podName -> containerName for restarts
}

// NewApp creates a new App instance with configuration
func NewApp(cfg *domain.Config) *App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	app := &App{
		styles:            DefaultStyles(),
		config:            cfg,
		viewState:         ViewKubeConfigSelect,
		connectionStatus:  domain.StatusDisconnected,
		spinner:           s,
		podDetails:        NewPodDetailsModel(DefaultStyles()),
		confirmDialog:     NewConfirmDialog(),
		notification:      NewNotification(),
		logViewer:         NewLogViewer(DefaultStyles()),
		containerSelector: NewContainerSelector(),
		passphraseInput:       NewPassphraseInput(),
		crictlLogViewer:       NewCrictlLogViewer(DefaultStyles()),
		helpScreen:            NewHelpScreen(),
		searchInput:           NewSearchInput(),
		deploymentDetails:     NewDeploymentDetailsModel(DefaultStyles()),
		serviceDetails:        NewServiceDetailsModel(DefaultStyles()),
		eventViewer:           NewEventViewer(DefaultStyles()),
		scaleDialog:           NewScaleDialog(),
		podMultiSelector:      NewPodMultiSelector(),
		multiPodLogViewer:     NewMultiPodLogViewer(DefaultStyles()),
	}

	// If only one kubeconfig, auto-select it
	if len(cfg.KubeConfigs) == 1 {
		app.selectedConfig = &cfg.KubeConfigs[0]
	}

	return app
}

// Init implements tea.Model
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.spinner.Tick}

	// Auto-connect if only one kubeconfig
	if a.selectedConfig != nil {
		a.viewState = ViewConnecting
		a.connectionStatus = domain.StatusConnecting
		cmds = append(cmds, a.connectToCluster(a.selectedConfig.Path))
	}

	return tea.Batch(cmds...)
}

// connectToCluster returns a command that connects to the cluster
func (a *App) connectToCluster(kubeconfigPath string) tea.Cmd {
	return func() tea.Msg {
		client, err := k8s.NewClient(kubeconfigPath)
		if err != nil {
			return connectResultMsg{err: err}
		}

		ctx := context.Background()
		if err := client.CheckConnection(ctx); err != nil {
			return connectResultMsg{err: err}
		}

		info, err := client.GetClusterInfo(ctx)
		if err != nil {
			return connectResultMsg{client: client, err: err}
		}

		return connectResultMsg{client: client, clusterInfo: info}
	}
}

// fetchNamespaces returns a command that fetches namespaces
func (a *App) fetchNamespaces() tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return namespacesResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		namespaces, err := a.k8sClient.GetNamespaces(ctx)
		if err != nil {
			return namespacesResultMsg{err: err}
		}

		return namespacesResultMsg{namespaces: namespaces}
	}
}

// fetchPods returns a command that fetches pods
func (a *App) fetchPods() tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return podsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		pods, err := a.k8sClient.GetPods(ctx, a.k8sClient.CurrentNamespace())
		if err != nil {
			return podsResultMsg{err: err}
		}

		return podsResultMsg{pods: pods}
	}
}

// fetchPodDetails returns a command that fetches pod details and events
func (a *App) fetchPodDetails(podName string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return podDetailsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		namespace := a.k8sClient.CurrentNamespace()

		pod, err := a.k8sClient.GetPod(ctx, namespace, podName)
		if err != nil {
			return podDetailsResultMsg{err: err}
		}

		events, err := a.k8sClient.GetPodEvents(ctx, namespace, podName)
		if err != nil {
			// Non-fatal: continue without events
			events = nil
		}

		return podDetailsResultMsg{pod: pod, events: events}
	}
}

// schedulePodRefresh returns a command that triggers a pod refresh after interval
func (a *App) schedulePodRefresh() tea.Cmd {
	return tea.Tick(podRefreshInterval, func(t time.Time) tea.Msg {
		return podRefreshTickMsg{}
	})
}

// scheduleEventRefresh returns a command that triggers an event refresh after interval
func (a *App) scheduleEventRefresh() tea.Cmd {
	return tea.Tick(podRefreshInterval, func(t time.Time) tea.Msg {
		return eventRefreshTickMsg{}
	})
}

// deletePod returns a command that deletes a pod
func (a *App) deletePod(podName string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return podDeleteResultMsg{podName: podName, err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		err := a.k8sClient.DeletePod(ctx, a.k8sClient.CurrentNamespace(), podName)
		return podDeleteResultMsg{podName: podName, err: err}
	}
}

// restartPod returns a command that restarts a pod (by deleting it)
func (a *App) restartPod(podName string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return podRestartResultMsg{podName: podName, err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		err := a.k8sClient.DeletePod(ctx, a.k8sClient.CurrentNamespace(), podName)
		return podRestartResultMsg{podName: podName, err: err}
	}
}

// fetchDeployments returns a command that fetches deployments
func (a *App) fetchDeployments() tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return deploymentsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		deployments, err := a.k8sClient.GetDeployments(ctx, a.k8sClient.CurrentNamespace())
		return deploymentsResultMsg{deployments: deployments, err: err}
	}
}

// fetchDeploymentDetails returns a command that fetches deployment details
func (a *App) fetchDeploymentDetails(name string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return deploymentDetailsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		deployment, err := a.k8sClient.GetDeployment(ctx, a.k8sClient.CurrentNamespace(), name)
		return deploymentDetailsResultMsg{deployment: deployment, err: err}
	}
}

// scaleDeployment returns a command that scales a deployment
func (a *App) scaleDeployment(name string, replicas int32) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return deploymentScaleResultMsg{deploymentName: name, err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		err := a.k8sClient.ScaleDeployment(ctx, a.k8sClient.CurrentNamespace(), name, replicas)
		return deploymentScaleResultMsg{deploymentName: name, replicas: replicas, err: err}
	}
}

// restartDeployment returns a command that restarts a deployment
func (a *App) restartDeployment(name string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return deploymentRestartResultMsg{deploymentName: name, err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		err := a.k8sClient.RestartDeployment(ctx, a.k8sClient.CurrentNamespace(), name)
		return deploymentRestartResultMsg{deploymentName: name, err: err}
	}
}

// deleteDeployment returns a command that deletes a deployment
func (a *App) deleteDeployment(name string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return deploymentDeleteResultMsg{deploymentName: name, err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		err := a.k8sClient.DeleteDeployment(ctx, a.k8sClient.CurrentNamespace(), name)
		return deploymentDeleteResultMsg{deploymentName: name, err: err}
	}
}

// fetchServices returns a command that fetches services
func (a *App) fetchServices() tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return servicesResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		services, err := a.k8sClient.GetServices(ctx, a.k8sClient.CurrentNamespace())
		return servicesResultMsg{services: services, err: err}
	}
}

// fetchServiceDetails returns a command that fetches service details
func (a *App) fetchServiceDetails(name string) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return serviceDetailsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		service, err := a.k8sClient.GetService(ctx, a.k8sClient.CurrentNamespace(), name)
		return serviceDetailsResultMsg{service: service, err: err}
	}
}

// fetchEvents returns a command that fetches events
func (a *App) fetchEvents() tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return eventsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		// Fetch all events - filtering is done in the viewer for better UX
		events, err := a.k8sClient.GetEvents(ctx, a.k8sClient.CurrentNamespace())
		return eventsResultMsg{events: events, err: err}
	}
}

// fetchMetrics returns a command that fetches pod metrics
func (a *App) fetchMetrics() tea.Cmd {
	return func() tea.Msg {
		if a.metricsClient == nil {
			return metricsResultMsg{err: fmt.Errorf("metrics not available")}
		}

		ctx := context.Background()
		metrics, err := a.metricsClient.GetPodMetrics(ctx, a.k8sClient.CurrentNamespace())
		return metricsResultMsg{metrics: metrics, err: err}
	}
}

// fetchContainers returns a command that fetches container names for a pod
func (a *App) fetchContainers(podName string) tea.Cmd {
	logger.Debug("fetchContainers called", "pod", podName)
	return func() tea.Msg {
		if a.k8sClient == nil {
			logger.Error("fetchContainers: k8sClient is nil")
			return containersResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		logger.Debug("Calling GetPodContainers", "pod", podName, "namespace", a.k8sClient.CurrentNamespace())
		containers, err := a.k8sClient.GetPodContainers(ctx, a.k8sClient.CurrentNamespace(), podName)
		if err != nil {
			logger.Error("GetPodContainers failed", "err", err)
		} else {
			logger.Debug("GetPodContainers returned containers", "count", len(containers))
		}
		return containersResultMsg{containers: containers, err: err}
	}
}

// fetchLogs returns a command that fetches logs for a pod
func (a *App) fetchLogs(podName, container string, tailLines int64, timestamps bool) tea.Cmd {
	return func() tea.Msg {
		if a.k8sClient == nil {
			return logsResultMsg{err: fmt.Errorf("not connected to cluster")}
		}

		ctx := context.Background()
		opts := k8s.LogOptions{
			Container:  container,
			TailLines:  tailLines,
			Timestamps: timestamps,
			Follow:     false,
		}

		logs, err := a.k8sClient.GetPodLogs(ctx, a.k8sClient.CurrentNamespace(), podName, opts)
		return logsResultMsg{logs: logs, err: err}
	}
}

// stopLogStream stops the current log stream
func (a *App) stopLogStream() {
	if a.logStreamCancel != nil {
		a.logStreamCancel()
		a.logStreamCancel = nil
	}
	a.logStreamActive = false
}

// Update implements tea.Model
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true

		// Calculate sidebar dimensions
		if a.width >= sidebarMinWidth {
			a.showSidebar = true
			a.sidebarWidth = a.width / 4
			if a.sidebarWidth < 24 {
				a.sidebarWidth = 24
			}
			if a.sidebarWidth > 30 {
				a.sidebarWidth = 30
			}
			a.contentWidth = a.width - 4 - a.sidebarWidth
		} else {
			a.showSidebar = false
			a.sidebarWidth = 0
			a.contentWidth = a.width - 4
		}

		cw := a.contentWidth
		// When sidebar is shown, content has more vertical space (no header)
		listH := a.height - 10
		viewH := a.height - 8
		logH := a.height - 10
		if a.showSidebar {
			listH = a.height - 8
			viewH = a.height - 6
			logH = a.height - 6
		}

		a.kubeConfigList = newKubeConfigList(
			a.config.KubeConfigs,
			cw,
			listH,
			a.styles,
		)
		a.namespaceList = newNamespaceList(nil, cw, listH, a.styles)
		a.podList = newPodList(nil, cw, listH, a.styles, a.metricsEnabled, a.podMetrics)
		a.podDetails.SetSize(cw, viewH)
		a.logViewer.SetSize(cw, logH)
		a.confirmDialog.SetWidth(a.width)
		a.notification.SetWidth(a.width)
		a.containerSelector.SetWidth(a.width)
		a.sshHostList = newSSHHostList(a.config.SSHHosts, cw, listH, a.styles)
		a.crictlContainerList = newCrictlContainerList(nil, cw, listH, a.styles)
		a.passphraseInput.SetWidth(a.width)
		a.crictlLogViewer.SetSize(cw, logH)
		a.helpScreen.SetSize(a.width, a.height)
		a.deploymentList = newDeploymentList(nil, cw, listH, a.styles)
		a.deploymentDetails.SetSize(cw, viewH)
		a.serviceList = newServiceList(nil, cw, listH, a.styles)
		a.serviceDetails.SetSize(cw, viewH)
		a.eventViewer.SetSize(cw, logH)
		a.scaleDialog.SetWidth(a.width)
		a.podMultiSelector.SetWidth(a.width)
		a.multiPodLogViewer.SetSize(cw, logH)
		return a, nil

	case connectResultMsg:
		return a.handleConnectResult(msg)

	case namespacesResultMsg:
		return a.handleNamespacesResult(msg)

	case podsResultMsg:
		return a.handlePodsResult(msg)

	case podDetailsResultMsg:
		return a.handlePodDetailsResult(msg)

	case podRefreshTickMsg:
		// Only refresh if we're on the pods view, dialog is not visible, and not filtering
		if a.viewState == ViewPods && a.k8sClient != nil && !a.confirmDialog.IsVisible() && !a.podList.SettingFilter() && a.podList.FilterState() == list.Unfiltered {
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		}
		// Reschedule even if we skip refresh (to check again later)
		if a.viewState == ViewPods && a.k8sClient != nil {
			return a, a.schedulePodRefresh()
		}
		return a, nil

	case eventRefreshTickMsg:
		// Only refresh if we're on the events view and following
		if a.viewState == ViewEvents && a.k8sClient != nil && a.eventViewer.IsFollowing() {
			return a, tea.Batch(a.fetchEvents(), a.scheduleEventRefresh())
		}
		// Reschedule even if we skip refresh (to check again later when following is re-enabled)
		if a.viewState == ViewEvents && a.k8sClient != nil {
			return a, a.scheduleEventRefresh()
		}
		return a, nil

	case podDeleteResultMsg:
		return a.handlePodDeleteResult(msg)

	case podRestartResultMsg:
		return a.handlePodRestartResult(msg)

	case notificationExpiredMsg:
		a.notification.Hide()
		return a, nil

	case containersResultMsg:
		return a.handleContainersResult(msg)

	case logsResultMsg:
		return a.handleLogsResult(msg)

	case logLineMsg:
		return a.handleLogLine(msg)

	case logStreamEndedMsg:
		return a.handleLogStreamEnded(msg)

	case multiPodLogLineMsg:
		return a.handleMultiPodLogLine(msg)

	case multiPodLogStreamEndedMsg:
		return a.handleMultiPodLogStreamEnded(msg)

	case sshConnectResultMsg:
		return a.handleSSHConnectResult(msg)

	case sshCrictlContainersMsg:
		return a.handleSSHCrictlContainers(msg)

	case sshNodeInfoMsg:
		return a.handleSSHNodeInfo(msg)

	case sshCrictlLogsMsg:
		return a.handleCrictlLogsResult(msg)

	case sshCrictlLogLineMsg:
		return a.handleCrictlLogLine(msg)

	case sshCrictlLogStreamEndedMsg:
		return a.handleCrictlLogStreamEnded(msg)

	// Deployment messages
	case deploymentsResultMsg:
		return a.handleDeploymentsResult(msg)

	case deploymentDetailsResultMsg:
		return a.handleDeploymentDetailsResult(msg)

	case deploymentScaleResultMsg:
		return a.handleDeploymentScaleResult(msg)

	case deploymentRestartResultMsg:
		return a.handleDeploymentRestartResult(msg)

	case deploymentDeleteResultMsg:
		return a.handleDeploymentDeleteResult(msg)

	// Service messages
	case servicesResultMsg:
		return a.handleServicesResult(msg)

	case serviceDetailsResultMsg:
		return a.handleServiceDetailsResult(msg)

	// Event messages
	case eventsResultMsg:
		return a.handleEventsResult(msg)

	// Metrics messages
	case metricsResultMsg:
		return a.handleMetricsResult(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	}

	// Route non-key messages to visible dialogs (huh forms produce internal
	// commands like nextGroupMsg that need to flow back through Update).
	if a.confirmDialog.IsVisible() {
		confirmed, cancelled, cmd := a.confirmDialog.Update(msg)
		if confirmed {
			action := a.confirmDialog.Action()
			targetName := a.confirmDialog.TargetName()
			a.confirmDialog.Hide()
			switch action {
			case ConfirmActionDeletePod:
				return a, a.deletePod(targetName)
			case ConfirmActionRestartPod:
				return a, a.restartPod(targetName)
			case ConfirmActionDeleteDeployment:
				return a, a.deleteDeployment(targetName)
			case ConfirmActionRestartDeployment:
				return a, a.restartDeployment(targetName)
			}
		}
		if cancelled {
			a.confirmDialog.Hide()
		}
		return a, cmd
	}
	if a.scaleDialog.IsVisible() {
		confirmed, cancelled, cmd := a.scaleDialog.Update(msg)
		if confirmed {
			deployName := a.scaleDialog.DeploymentName()
			replicas := a.scaleDialog.TargetReplicas()
			a.scaleDialog.Hide()
			return a, a.scaleDeployment(deployName, replicas)
		}
		if cancelled {
			a.scaleDialog.Hide()
		}
		return a, cmd
	}
	if a.containerSelector.IsVisible() {
		selected, cancelled, cmd := a.containerSelector.Update(msg)
		if selected {
			container := a.containerSelector.SelectedContainer()
			a.containerSelector.Hide()
			a.logViewer.SetContainer(container)
			a.stopLogStream()
			a.loading = true
			return a, a.fetchLogs(
				a.logViewer.PodName(),
				container,
				a.logViewer.TailLines(),
				a.logViewer.Timestamps(),
			)
		}
		if cancelled {
			a.containerSelector.Hide()
		}
		return a, cmd
	}
	if a.podMultiSelector.IsVisible() {
		confirmed, cancelled, cmd := a.podMultiSelector.Update(msg)
		if confirmed {
			selectedPods := a.podMultiSelector.SelectedPods()
			a.podMultiSelector.Hide()
			if len(selectedPods) > 0 {
				return a, a.startMultiPodStreaming(selectedPods)
			}
		}
		if cancelled {
			a.podMultiSelector.Hide()
		}
		return a, cmd
	}
	if a.passphraseInput.IsVisible() {
		passphrase, submitted, cancelled, cmd := a.passphraseInput.Update(msg)
		if submitted {
			a.passphraseInput.Hide()
			if a.sshClient != nil && a.selectedSSHHost != nil {
				a.sshClient.SetPassphrase(passphrase)
				a.loading = true
				return a, a.retrySSHConnection()
			}
		}
		if cancelled {
			a.passphraseInput.Hide()
			a.viewState = ViewSSHHosts
		}
		return a, cmd
	}

	// Update child components based on view state
	switch a.viewState {
	case ViewKubeConfigSelect:
		var cmd tea.Cmd
		a.kubeConfigList, cmd = a.kubeConfigList.Update(msg)
		return a, cmd
	case ViewNamespaces:
		var cmd tea.Cmd
		a.namespaceList, cmd = a.namespaceList.Update(msg)
		return a, cmd
	case ViewPods:
		var cmd tea.Cmd
		a.podList, cmd = a.podList.Update(msg)
		return a, cmd
	case ViewPodDetails:
		var cmd tea.Cmd
		a.podDetails, cmd = a.podDetails.Update(msg)
		return a, cmd
	case ViewLogs:
		var cmd tea.Cmd
		a.logViewer, cmd = a.logViewer.Update(msg)
		return a, cmd
	case ViewMultiPodLogs:
		var cmd tea.Cmd
		a.multiPodLogViewer, cmd = a.multiPodLogViewer.Update(msg)
		return a, cmd
	case ViewSSHHosts:
		var cmd tea.Cmd
		a.sshHostList, cmd = a.sshHostList.Update(msg)
		return a, cmd
	case ViewCrictlContainers:
		var cmd tea.Cmd
		a.crictlContainerList, cmd = a.crictlContainerList.Update(msg)
		return a, cmd
	case ViewCrictlLogs:
		var cmd tea.Cmd
		a.crictlLogViewer, cmd = a.crictlLogViewer.Update(msg)
		return a, cmd
	case ViewDeployments:
		var cmd tea.Cmd
		a.deploymentList, cmd = a.deploymentList.Update(msg)
		return a, cmd
	case ViewDeploymentDetails:
		var cmd tea.Cmd
		a.deploymentDetails, cmd = a.deploymentDetails.Update(msg)
		return a, cmd
	case ViewServices:
		var cmd tea.Cmd
		a.serviceList, cmd = a.serviceList.Update(msg)
		return a, cmd
	case ViewServiceDetails:
		var cmd tea.Cmd
		a.serviceDetails, cmd = a.serviceDetails.Update(msg)
		return a, cmd
	case ViewEvents:
		var cmd tea.Cmd
		a.eventViewer, cmd = a.eventViewer.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a *App) handleConnectResult(msg connectResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.err = msg.err
		a.connectionStatus = domain.StatusError
		a.viewState = ViewMain
		return a, nil
	}

	a.k8sClient = msg.client
	a.clusterInfo = msg.clusterInfo
	a.connectionStatus = domain.StatusConnected
	a.viewState = ViewNamespaces
	a.err = nil
	a.loading = true

	// Initialize metrics client (optional - may not be available)
	if a.selectedConfig != nil {
		metricsClient, err := k8s.NewMetricsClient(a.selectedConfig.Path)
		if err == nil {
			// Check if metrics server is actually available
			ctx := context.Background()
			if metricsClient.CheckMetricsAvailable(ctx) {
				a.metricsClient = metricsClient
				a.metricsAvailable = true
				logger.Debug("Metrics server available")
			} else {
				logger.Debug("Metrics server not available")
			}
		} else {
			logger.Debug("Failed to create metrics client", "err", err)
		}
	}

	// Fetch namespaces after successful connection
	return a, a.fetchNamespaces()
}

func (a *App) handleNamespacesResult(msg namespacesResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.namespaceCount = len(msg.namespaces)
	updateNamespaceList(&a.namespaceList, msg.namespaces)
	a.err = nil
	return a, nil
}

func (a *App) handlePodsResult(msg podsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.podCount = len(msg.pods)
	a.pods = msg.pods

	// Don't update list while filtering - it breaks the filter state
	if !a.podList.SettingFilter() && a.podList.FilterState() == list.Unfiltered {
		updatePodList(&a.podList, msg.pods)
	}
	a.err = nil
	return a, nil
}

func (a *App) handlePodDetailsResult(msg podDetailsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.podDetails.SetPod(msg.pod, msg.events)
	a.err = nil
	return a, nil
}

func (a *App) handlePodDeleteResult(msg podDeleteResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Failed to delete pod: %v", msg.err),
			NotificationError,
		)
		return a, notifCmd
	}

	notifCmd := a.notification.Show(
		fmt.Sprintf("Pod '%s' deleted successfully", msg.podName),
		NotificationSuccess,
	)

	// Go back to pods view and refresh
	a.viewState = ViewPods
	a.selectedPodName = ""
	return a, tea.Batch(notifCmd, a.fetchPods(), a.schedulePodRefresh())
}

func (a *App) handlePodRestartResult(msg podRestartResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Failed to restart pod: %v", msg.err),
			NotificationError,
		)
		return a, notifCmd
	}

	notifCmd := a.notification.Show(
		fmt.Sprintf("Pod '%s' restarting...", msg.podName),
		NotificationSuccess,
	)

	// Go back to pods view and refresh
	a.viewState = ViewPods
	a.selectedPodName = ""
	return a, tea.Batch(notifCmd, a.fetchPods(), a.schedulePodRefresh())
}

// Deployment result handlers
func (a *App) handleDeploymentsResult(msg deploymentsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.deploymentCount = len(msg.deployments)
	updateDeploymentList(&a.deploymentList, msg.deployments)
	a.err = nil
	return a, nil
}

func (a *App) handleDeploymentDetailsResult(msg deploymentDetailsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.deploymentDetails.SetDeployment(msg.deployment)
	a.err = nil
	return a, nil
}

func (a *App) handleDeploymentScaleResult(msg deploymentScaleResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Failed to scale deployment: %v", msg.err),
			NotificationError,
		)
		return a, notifCmd
	}

	notifCmd := a.notification.Show(
		fmt.Sprintf("Deployment '%s' scaled to %d replicas", msg.deploymentName, msg.replicas),
		NotificationSuccess,
	)

	return a, tea.Batch(notifCmd, a.fetchDeployments())
}

func (a *App) handleDeploymentRestartResult(msg deploymentRestartResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Failed to restart deployment: %v", msg.err),
			NotificationError,
		)
		return a, notifCmd
	}

	notifCmd := a.notification.Show(
		fmt.Sprintf("Deployment '%s' restarting...", msg.deploymentName),
		NotificationSuccess,
	)

	a.viewState = ViewDeployments
	a.selectedDeployName = ""
	return a, tea.Batch(notifCmd, a.fetchDeployments())
}

func (a *App) handleDeploymentDeleteResult(msg deploymentDeleteResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Failed to delete deployment: %v", msg.err),
			NotificationError,
		)
		return a, notifCmd
	}

	notifCmd := a.notification.Show(
		fmt.Sprintf("Deployment '%s' deleted", msg.deploymentName),
		NotificationSuccess,
	)

	a.viewState = ViewDeployments
	a.selectedDeployName = ""
	return a, tea.Batch(notifCmd, a.fetchDeployments())
}

// Service result handlers
func (a *App) handleServicesResult(msg servicesResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.serviceCount = len(msg.services)
	updateServiceList(&a.serviceList, msg.services)
	a.err = nil
	return a, nil
}

func (a *App) handleServiceDetailsResult(msg serviceDetailsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.serviceDetails.SetService(msg.service)
	a.err = nil
	return a, nil
}

// Event result handler
func (a *App) handleEventsResult(msg eventsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.eventViewer.SetEvents(msg.events)
	a.err = nil
	return a, nil
}

// Metrics result handler
func (a *App) handleMetricsResult(msg metricsResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Silently fail - metrics are optional
		a.podMetrics = nil
		return a, nil
	}

	a.podMetrics = msg.metrics

	// Rebuild pod list with new metrics data if metrics are enabled
	if a.metricsEnabled && a.viewState == ViewPods {
		a.podList = newPodList(
			nil,
			a.podList.Width(),
			a.podList.Height(),
			a.styles,
			a.metricsEnabled,
			a.podMetrics,
		)
		if a.pods != nil {
			updatePodList(&a.podList, a.pods)
		}
	}

	return a, nil
}

func (a *App) handleContainersResult(msg containersResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		logger.Error("Failed to get containers", "pod", a.selectedPodName, "err", msg.err)
		a.err = msg.err
		return a, nil
	}

	logger.Debug("Got containers for pod", "count", len(msg.containers), "pod", a.selectedPodName, "containers", msg.containers)

	// Set up log viewer with pod and containers
	a.logViewer.SetPod(a.selectedPodName, a.k8sClient.CurrentNamespace(), msg.containers)
	a.viewState = ViewLogs

	logger.Debug("Switching to ViewLogs, fetching logs", "container", a.logViewer.Container())

	// Fetch initial logs
	return a, a.fetchLogs(
		a.selectedPodName,
		a.logViewer.Container(),
		a.logViewer.TailLines(),
		a.logViewer.Timestamps(),
	)
}

func (a *App) handleLogsResult(msg logsResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		logger.Error("Failed to get logs", "err", msg.err)
		a.err = msg.err
		return a, nil
	}

	logLen := len(msg.logs)
	logger.Debug("Received logs", "bytes", logLen)

	a.logViewer.SetLogs(msg.logs)
	a.err = nil

	// Don't auto-start streaming - user must press 'f' to follow
	return a, nil
}

func (a *App) handleLogLine(msg logLineMsg) (tea.Model, tea.Cmd) {
	if a.viewState != ViewLogs {
		return a, nil
	}

	a.logViewer.AppendLog(msg.line)

	// Continue reading from stream if active
	if a.logStreamActive && a.logLineChan != nil {
		return a, a.waitForLogLine(a.logLineChan)
	}

	return a, nil
}

func (a *App) handleLogStreamEnded(msg logStreamEndedMsg) (tea.Model, tea.Cmd) {
	a.logStreamActive = false

	// If context was cancelled (user stopped follow), just return
	if msg.err == context.Canceled {
		return a, nil
	}

	// If follow mode is still enabled and we're on logs view, auto-restart the stream
	if a.logViewer.IsFollowing() && a.viewState == ViewLogs {
		logger.Debug("Log stream ended, auto-restarting (follow still enabled)")
		return a, a.startLogStreaming()
	}

	// Show notification if there was an error
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Log stream ended: %v", msg.err),
			NotificationInfo,
		)
		return a, notifCmd
	}

	return a, nil
}

// startLogStreaming starts the log streaming using a subscription pattern
func (a *App) startLogStreaming() tea.Cmd {
	if a.k8sClient == nil {
		return nil
	}

	// Stop any existing stream
	a.stopLogStream()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	a.logStreamCancel = cancel
	a.logStreamActive = true

	opts := k8s.LogOptions{
		Container:  a.logViewer.Container(),
		TailLines:  0, // Don't re-fetch tail when streaming
		Timestamps: a.logViewer.Timestamps(),
		Follow:     true,
	}

	lineChan := make(chan string, 100)
	a.logLineChan = lineChan

	// Start streaming in a goroutine
	go func() {
		defer close(lineChan)
		_ = a.k8sClient.StreamPodLogs(
			ctx,
			a.k8sClient.CurrentNamespace(),
			a.logViewer.PodName(),
			opts,
			lineChan,
		)
	}()

	// Return a command that reads from the channel
	return a.waitForLogLine(lineChan)
}

// waitForLogLine returns a command that waits for the next log line
func (a *App) waitForLogLine(lineChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lineChan
		if !ok {
			return logStreamEndedMsg{}
		}
		return logLineMsg{line: line}
	}
}

// SSH-related commands and handlers

// connectToSSHHost connects to an SSH host and returns a command
func (a *App) connectToSSHHost(host domain.SSHHost) tea.Cmd {
	// Close existing connection first
	a.closeSSHConnection()

	// Create and store client
	a.sshClient = ssh.NewClient(host)
	a.selectedSSHHost = &host

	return a.retrySSHConnection()
}

// retrySSHConnection retries SSH connection using the existing client (useful after setting passphrase)
func (a *App) retrySSHConnection() tea.Cmd {
	return func() tea.Msg {
		if a.sshClient == nil {
			return sshConnectResultMsg{err: fmt.Errorf("no SSH client")}
		}

		ctx := context.Background()

		if err := a.sshClient.Connect(ctx); err != nil {
			return sshConnectResultMsg{err: err}
		}

		if err := a.sshClient.TestConnection(ctx); err != nil {
			a.sshClient.Close()
			return sshConnectResultMsg{err: err}
		}

		return sshConnectResultMsg{err: nil}
	}
}

// fetchCrictlContainers returns a command that fetches crictl containers
func (a *App) fetchCrictlContainers() tea.Cmd {
	return func() tea.Msg {
		if a.sshClient == nil {
			return sshCrictlContainersMsg{err: fmt.Errorf("not connected to SSH host")}
		}

		ctx := context.Background()
		containers, err := a.sshClient.ListContainers(ctx)
		return sshCrictlContainersMsg{containers: containers, err: err}
	}
}

// fetchNodeInfo returns a command that fetches node information
func (a *App) fetchNodeInfo() tea.Cmd {
	return func() tea.Msg {
		if a.sshClient == nil {
			return sshNodeInfoMsg{err: fmt.Errorf("not connected to SSH host")}
		}

		ctx := context.Background()
		info, err := a.sshClient.GetNodeInfo(ctx)
		return sshNodeInfoMsg{info: info, err: err}
	}
}

func (a *App) handleSSHConnectResult(msg sshConnectResultMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		// Check if passphrase is required
		if msg.err == ssh.ErrPassphraseRequired {
			// Show passphrase input dialog
			hostName := ""
			if a.selectedSSHHost != nil {
				hostName = a.selectedSSHHost.Name
			}
			cmd := a.passphraseInput.Show(hostName)
			a.viewState = ViewSSHConnecting
			return a, cmd
		}
		a.err = msg.err
		a.viewState = ViewSSHHosts
		return a, nil
	}

	// Connection successful - switch to crictl containers view
	a.viewState = ViewCrictlContainers
	a.err = nil
	a.loading = true

	// Fetch containers and node info
	return a, tea.Batch(a.fetchCrictlContainers(), a.fetchNodeInfo())
}

func (a *App) handleSSHCrictlContainers(msg sshCrictlContainersMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}

	a.crictlContainers = msg.containers
	updateCrictlContainerList(&a.crictlContainerList, msg.containers)
	a.err = nil
	return a, nil
}

func (a *App) handleSSHNodeInfo(msg sshNodeInfoMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Non-fatal, just log
		logger.Debug("Failed to get node info", "err", msg.err)
		return a, nil
	}

	a.nodeInfo = msg.info
	return a, nil
}

// closeSSHConnection closes the current SSH connection
func (a *App) closeSSHConnection() {
	if a.sshClient != nil {
		a.sshClient.Close()
		a.sshClient = nil
	}
	a.selectedSSHHost = nil
	a.nodeInfo = nil
	a.crictlContainers = nil
}

// fetchCrictlLogs returns a command that fetches crictl container logs
func (a *App) fetchCrictlLogs() tea.Cmd {
	return func() tea.Msg {
		if a.sshClient == nil || a.selectedCrictlContainer == nil {
			return sshCrictlLogsMsg{err: fmt.Errorf("not connected or no container selected")}
		}

		ctx := context.Background()
		opts := ssh.CrictlLogOptions{
			TailLines:  a.crictlLogViewer.TailLines(),
			Timestamps: a.crictlLogViewer.Timestamps(),
		}

		logs, err := a.sshClient.ContainerLogs(ctx, a.selectedCrictlContainer.ContainerID, opts)
		return sshCrictlLogsMsg{logs: logs, err: err}
	}
}

// stopCrictlLogStream stops the current crictl log stream
func (a *App) stopCrictlLogStream() {
	if a.crictlLogStreamCancel != nil {
		a.crictlLogStreamCancel()
		a.crictlLogStreamCancel = nil
	}
	a.crictlLogStreamActive = false
}

// startCrictlLogStreaming starts the crictl log streaming
func (a *App) startCrictlLogStreaming() tea.Cmd {
	if a.sshClient == nil || a.selectedCrictlContainer == nil {
		return nil
	}

	// Stop any existing stream
	a.stopCrictlLogStream()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	a.crictlLogStreamCancel = cancel
	a.crictlLogStreamActive = true

	opts := ssh.CrictlLogOptions{
		TailLines:  0, // Don't re-fetch tail when streaming
		Timestamps: a.crictlLogViewer.Timestamps(),
		Follow:     true,
	}

	lineChan := make(chan string, 100)
	a.crictlLogLineChan = lineChan

	containerID := a.selectedCrictlContainer.ContainerID

	// Start streaming in a goroutine
	go func() {
		defer close(lineChan)
		_ = a.sshClient.StreamContainerLogs(ctx, containerID, opts, lineChan)
	}()

	// Return a command that reads from the channel
	return a.waitForCrictlLogLine(lineChan)
}

// waitForCrictlLogLine returns a command that waits for the next crictl log line
func (a *App) waitForCrictlLogLine(lineChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lineChan
		if !ok {
			return sshCrictlLogStreamEndedMsg{}
		}
		return sshCrictlLogLineMsg{line: line}
	}
}

func (a *App) handleCrictlLogsResult(msg sshCrictlLogsMsg) (tea.Model, tea.Cmd) {
	a.loading = false

	if msg.err != nil {
		logger.Error("Failed to get crictl logs", "err", msg.err)
		a.err = msg.err
		return a, nil
	}

	a.crictlLogViewer.SetLogs(msg.logs)
	a.err = nil
	return a, nil
}

func (a *App) handleCrictlLogLine(msg sshCrictlLogLineMsg) (tea.Model, tea.Cmd) {
	if a.viewState != ViewCrictlLogs {
		return a, nil
	}

	a.crictlLogViewer.AppendLog(msg.line)

	// Continue reading from stream if active
	if a.crictlLogStreamActive && a.crictlLogLineChan != nil {
		return a, a.waitForCrictlLogLine(a.crictlLogLineChan)
	}

	return a, nil
}

func (a *App) handleCrictlLogStreamEnded(msg sshCrictlLogStreamEndedMsg) (tea.Model, tea.Cmd) {
	a.crictlLogStreamActive = false

	// If context was cancelled (user stopped follow), just return
	if msg.err == context.Canceled {
		return a, nil
	}

	// If follow mode is still enabled and we're on logs view, auto-restart the stream
	if a.crictlLogViewer.IsFollowing() && a.viewState == ViewCrictlLogs {
		logger.Debug("Crictl log stream ended, auto-restarting (follow still enabled)")
		return a, a.startCrictlLogStreaming()
	}

	// Show notification if there was an error
	if msg.err != nil {
		notifCmd := a.notification.Show(
			fmt.Sprintf("Log stream ended: %v", msg.err),
			NotificationInfo,
		)
		return a, notifCmd
	}

	return a, nil
}

func (a *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	logger.Debug("Key pressed", "key", key, "viewState", a.viewState)

	// Don't handle keys during connection
	if a.viewState == ViewConnecting {
		if key == "ctrl+c" {
			return a, tea.Quit
		}
		return a, nil
	}

	// Handle confirmation dialog if visible
	if a.confirmDialog.IsVisible() {
		confirmed, cancelled, cmd := a.confirmDialog.Update(msg)
		if confirmed {
			action := a.confirmDialog.Action()
			targetName := a.confirmDialog.TargetName()
			a.confirmDialog.Hide()

			switch action {
			case ConfirmActionDeletePod:
				return a, a.deletePod(targetName)
			case ConfirmActionRestartPod:
				return a, a.restartPod(targetName)
			case ConfirmActionDeleteDeployment:
				return a, a.deleteDeployment(targetName)
			case ConfirmActionRestartDeployment:
				return a, a.restartDeployment(targetName)
			}
		}
		if cancelled {
			a.confirmDialog.Hide()
		}
		return a, cmd
	}

	// Handle scale dialog if visible
	if a.scaleDialog.IsVisible() {
		confirmed, cancelled, cmd := a.scaleDialog.Update(msg)
		if confirmed {
			deployName := a.scaleDialog.DeploymentName()
			replicas := a.scaleDialog.TargetReplicas()
			a.scaleDialog.Hide()
			return a, a.scaleDeployment(deployName, replicas)
		}
		if cancelled {
			a.scaleDialog.Hide()
		}
		return a, cmd
	}

	// Handle help screen if visible
	if a.helpScreen.IsVisible() {
		if key == "?" || key == "esc" {
			a.helpScreen.Hide()
		}
		return a, nil
	}

	// Handle container selector if visible
	if a.containerSelector.IsVisible() {
		selected, cancelled, cmd := a.containerSelector.Update(msg)
		if selected {
			container := a.containerSelector.SelectedContainer()
			a.containerSelector.Hide()
			a.logViewer.SetContainer(container)
			a.stopLogStream()
			a.loading = true
			return a, a.fetchLogs(
				a.logViewer.PodName(),
				container,
				a.logViewer.TailLines(),
				a.logViewer.Timestamps(),
			)
		}
		if cancelled {
			a.containerSelector.Hide()
		}
		return a, cmd
	}

	// Handle pod multi-selector if visible
	if a.podMultiSelector.IsVisible() {
		confirmed, cancelled, cmd := a.podMultiSelector.Update(msg)
		if confirmed {
			selectedPods := a.podMultiSelector.SelectedPods()
			a.podMultiSelector.Hide()
			if len(selectedPods) > 0 {
				return a, a.startMultiPodStreaming(selectedPods)
			}
		}
		if cancelled {
			a.podMultiSelector.Hide()
		}
		return a, cmd
	}

	// Handle passphrase input if visible
	if a.passphraseInput.IsVisible() {
		passphrase, submitted, cancelled, cmd := a.passphraseInput.Update(msg)
		if submitted {
			a.passphraseInput.Hide()
			if a.sshClient != nil && a.selectedSSHHost != nil {
				// Set passphrase on existing client and retry connection
				a.sshClient.SetPassphrase(passphrase)
				a.loading = true
				return a, a.retrySSHConnection()
			}
		}
		if cancelled {
			a.passphraseInput.Hide()
			a.viewState = ViewSSHHosts
		}
		return a, cmd
	}

	// Handle search input if visible (in log views)
	if a.searchInput.IsVisible() {
		query, submitted, cancelled, cmd := a.searchInput.Update(msg)
		if submitted || cancelled {
			a.searchInput.Hide()
			if cancelled {
				// Clear search
				if a.viewState == ViewLogs {
					a.logViewer.SetSearchQuery("")
				} else if a.viewState == ViewCrictlLogs {
					a.crictlLogViewer.SetSearchQuery("")
				}
			}
		} else {
			// Update search as user types
			if a.viewState == ViewLogs {
				a.logViewer.SetSearchQuery(query)
				a.searchInput.SetMatchCount(a.logViewer.MatchCount())
			} else if a.viewState == ViewCrictlLogs {
				a.crictlLogViewer.SetSearchQuery(query)
				a.searchInput.SetMatchCount(a.crictlLogViewer.MatchCount())
			}
		}
		return a, cmd
	}

	// Handle filter mode for lists
	if a.viewState == ViewKubeConfigSelect && a.kubeConfigList.SettingFilter() {
		var cmd tea.Cmd
		a.kubeConfigList, cmd = a.kubeConfigList.Update(msg)
		return a, cmd
	}
	if a.viewState == ViewNamespaces && a.namespaceList.SettingFilter() {
		var cmd tea.Cmd
		a.namespaceList, cmd = a.namespaceList.Update(msg)
		return a, cmd
	}
	if a.viewState == ViewPods && a.podList.SettingFilter() {
		var cmd tea.Cmd
		a.podList, cmd = a.podList.Update(msg)
		return a, cmd
	}
	if a.viewState == ViewSSHHosts && a.sshHostList.SettingFilter() {
		var cmd tea.Cmd
		a.sshHostList, cmd = a.sshHostList.Update(msg)
		return a, cmd
	}
	if a.viewState == ViewCrictlContainers && a.crictlContainerList.SettingFilter() {
		var cmd tea.Cmd
		a.crictlContainerList, cmd = a.crictlContainerList.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit

	case "?":
		// Show help screen (except during connection)
		if a.viewState != ViewConnecting && a.viewState != ViewSSHConnecting {
			a.helpScreen.Toggle()
			return a, nil
		}

	case "q":
		switch a.viewState {
		case ViewMain, ViewNamespaces, ViewPods, ViewPodDetails, ViewLogs, ViewMultiPodLogs, ViewSSHHosts, ViewCrictlContainers, ViewCrictlLogs, ViewNodeInfo:
			a.stopLogStream()           // Clean up any active log stream
			a.stopMultiPodStreams()      // Clean up multi-pod log streams
			a.stopCrictlLogStream()     // Clean up crictl log stream
			a.closeSSHConnection()      // Clean up SSH connection
			return a, tea.Quit
		case ViewKubeConfigSelect:
			return a, tea.Quit
		}

	case "enter":
		switch a.viewState {
		case ViewKubeConfigSelect:
			if item, ok := a.kubeConfigList.SelectedItem().(kubeConfigItem); ok {
				a.selectedConfig = &item.kubeConfig
				a.viewState = ViewConnecting
				a.connectionStatus = domain.StatusConnecting
				a.err = nil
				return a, a.connectToCluster(item.kubeConfig.Path)
			}
		case ViewNamespaces:
			if item, ok := a.namespaceList.SelectedItem().(namespaceItem); ok {
				a.k8sClient.SetNamespace(item.namespace.Name)
				a.clusterInfo.Namespace = item.namespace.Name
				a.viewState = ViewPods
				a.loading = true
				// Fetch pods and start auto-refresh
				return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
			}
		case ViewPods:
			if item, ok := a.podList.SelectedItem().(podItem); ok {
				a.selectedPodName = item.pod.Name
				a.viewState = ViewPodDetails
				a.loading = true
				return a, a.fetchPodDetails(item.pod.Name)
			}
		case ViewSSHHosts:
			if item, ok := a.sshHostList.SelectedItem().(sshHostItem); ok {
				a.viewState = ViewSSHConnecting
				a.loading = true
				a.err = nil
				return a, a.connectToSSHHost(item.host)
			}
		case ViewCrictlContainers:
			if item, ok := a.crictlContainerList.SelectedItem().(crictlContainerItem); ok {
				container := item.container
				a.selectedCrictlContainer = &container
				nodeName := ""
				if a.selectedSSHHost != nil {
					nodeName = a.selectedSSHHost.Name
				}
				a.crictlLogViewer.SetContainer(container.ContainerID, container.Name, nodeName)
				a.viewState = ViewCrictlLogs
				a.loading = true
				return a, a.fetchCrictlLogs()
			}
		case ViewDeployments:
			if item, ok := a.deploymentList.SelectedItem().(deploymentItem); ok {
				a.selectedDeployName = item.deployment.Name
				a.viewState = ViewDeploymentDetails
				a.loading = true
				return a, a.fetchDeploymentDetails(item.deployment.Name)
			}
		case ViewServices:
			if item, ok := a.serviceList.SelectedItem().(serviceItem); ok {
				a.selectedServiceName = item.service.Name
				a.viewState = ViewServiceDetails
				a.loading = true
				return a, a.fetchServiceDetails(item.service.Name)
			}
		}

	case "r":
		// Refresh
		switch a.viewState {
		case ViewNamespaces:
			if a.k8sClient != nil {
				a.loading = true
				return a, a.fetchNamespaces()
			}
		case ViewPods:
			if a.k8sClient != nil {
				a.loading = true
				return a, a.fetchPods()
			}
		case ViewPodDetails:
			if a.k8sClient != nil && a.selectedPodName != "" {
				a.loading = true
				return a, a.fetchPodDetails(a.selectedPodName)
			}
		case ViewLogs:
			if a.k8sClient != nil {
				a.stopLogStream()
				a.loading = true
				return a, a.fetchLogs(
					a.logViewer.PodName(),
					a.logViewer.Container(),
					a.logViewer.TailLines(),
					a.logViewer.Timestamps(),
				)
			}
		case ViewMain:
			if a.selectedConfig != nil && a.connectionStatus != domain.StatusConnected {
				a.viewState = ViewConnecting
				a.connectionStatus = domain.StatusConnecting
				a.err = nil
				return a, a.connectToCluster(a.selectedConfig.Path)
			}
		case ViewCrictlContainers:
			if a.sshClient != nil {
				a.loading = true
				return a, a.fetchCrictlContainers()
			}
		case ViewCrictlLogs:
			if a.sshClient != nil && a.selectedCrictlContainer != nil {
				a.stopCrictlLogStream()
				a.loading = true
				return a, a.fetchCrictlLogs()
			}
		case ViewDeployments:
			if a.k8sClient != nil {
				a.loading = true
				return a, a.fetchDeployments()
			}
		case ViewDeploymentDetails:
			if a.k8sClient != nil && a.selectedDeployName != "" {
				a.loading = true
				return a, a.fetchDeploymentDetails(a.selectedDeployName)
			}
		case ViewServices:
			if a.k8sClient != nil {
				a.loading = true
				return a, a.fetchServices()
			}
		case ViewServiceDetails:
			if a.k8sClient != nil && a.selectedServiceName != "" {
				a.loading = true
				return a, a.fetchServiceDetails(a.selectedServiceName)
			}
		case ViewEvents:
			if a.k8sClient != nil {
				a.loading = true
				return a, tea.Batch(a.fetchEvents(), a.scheduleEventRefresh())
			}
		}

	case "l":
		// Go to logs
		if a.viewState == ViewPodDetails && a.selectedPodName != "" {
			// From pod details - pod already selected
			logger.Debug("Opening logs from pod details", "pod", a.selectedPodName)
			a.logSourceView = ViewPodDetails
			a.loading = true
			return a, a.fetchContainers(a.selectedPodName)
		}
		if a.viewState == ViewPods {
			// From pods list - get selected pod
			if item, ok := a.podList.SelectedItem().(podItem); ok {
				a.selectedPodName = item.pod.Name
				logger.Debug("Opening logs from pods list", "pod", a.selectedPodName)
				a.logSourceView = ViewPods
				a.loading = true
				return a, a.fetchContainers(item.pod.Name)
			}
			logger.Warn("No pod selected in pods list")
		}

	case "L":
		// Multi-pod log streaming (Shift+L)
		if a.viewState == ViewPods && a.k8sClient != nil {
			items := a.podList.Items()
			pods := make([]domain.Pod, 0, len(items))
			for _, item := range items {
				if pi, ok := item.(podItem); ok {
					pods = append(pods, pi.pod)
				}
			}
			if len(pods) > 0 {
				return a, a.podMultiSelector.Show(pods)
			}
		}

	case "d":
		// Delete pod
		if a.viewState == ViewPodDetails && a.selectedPodName != "" {
			return a, a.confirmDialog.Show(ConfirmActionDeletePod, a.selectedPodName)
		}
		// Also allow deletion from pods list view
		if a.viewState == ViewPods {
			if item, ok := a.podList.SelectedItem().(podItem); ok {
				return a, a.confirmDialog.Show(ConfirmActionDeletePod, item.pod.Name)
			}
		}

	case "R":
		// Restart pod (Shift+R)
		if a.viewState == ViewPodDetails && a.selectedPodName != "" {
			return a, a.confirmDialog.Show(ConfirmActionRestartPod, a.selectedPodName)
		}
		// Also allow restart from pods list view
		if a.viewState == ViewPods {
			if item, ok := a.podList.SelectedItem().(podItem); ok {
				return a, a.confirmDialog.Show(ConfirmActionRestartPod, item.pod.Name)
			}
		}

	case "f":
		// Toggle follow mode in log viewer
		if a.viewState == ViewLogs {
			following := a.logViewer.ToggleFollowing()
			if following {
				// Start streaming
				return a, a.startLogStreaming()
			} else {
				// Stop streaming
				a.stopLogStream()
			}
			return a, nil
		}
		// Toggle follow mode in crictl log viewer
		if a.viewState == ViewCrictlLogs {
			following := a.crictlLogViewer.ToggleFollowing()
			if following {
				return a, a.startCrictlLogStreaming()
			} else {
				a.stopCrictlLogStream()
			}
			return a, nil
		}
		// Toggle follow mode in event viewer
		if a.viewState == ViewEvents {
			a.eventViewer.ToggleFollowing()
			return a, nil
		}
		// Toggle follow mode in multi-pod log viewer
		if a.viewState == ViewMultiPodLogs {
			a.multiPodLogViewer.ToggleFollowing()
			return a, nil
		}

	case "t":
		// Toggle timestamps in log viewer
		if a.viewState == ViewLogs {
			a.logViewer.ToggleTimestamps()
			a.stopLogStream()
			a.loading = true
			return a, a.fetchLogs(
				a.logViewer.PodName(),
				a.logViewer.Container(),
				a.logViewer.TailLines(),
				a.logViewer.Timestamps(),
			)
		}
		// Toggle timestamps in crictl log viewer
		if a.viewState == ViewCrictlLogs && a.selectedCrictlContainer != nil {
			a.crictlLogViewer.ToggleTimestamps()
			a.stopCrictlLogStream()
			a.loading = true
			return a, a.fetchCrictlLogs()
		}

	case "m":
		// Toggle metrics display in pod list
		if a.viewState == ViewPods {
			if a.metricsClient != nil {
				a.metricsEnabled = !a.metricsEnabled
				// Rebuild pod list with new metrics setting
				a.podList = newPodList(
					nil,
					a.podList.Width(),
					a.podList.Height(),
					a.styles,
					a.metricsEnabled,
					a.podMetrics,
				)
				// Restore items
				if a.pods != nil {
					updatePodList(&a.podList, a.pods)
				}
				// Fetch metrics if enabling
				if a.metricsEnabled && a.podMetrics == nil {
					return a, a.fetchMetrics()
				}
			} else {
				a.notification.Show("Metrics not available (metrics-server not installed)", NotificationWarning)
			}
			return a, nil
		}

	case "c":
		// Change container in log viewer
		if a.viewState == ViewLogs && len(a.logViewer.Containers()) > 1 {
			return a, a.containerSelector.Show(a.logViewer.Containers(), a.logViewer.Container())
		}

	case "/":
		// Start search in log views
		if a.viewState == ViewLogs || a.viewState == ViewCrictlLogs {
			a.searchInput.Show()
			return a, nil
		}

	case "s":
		// Scale deployment
		if a.viewState == ViewDeployments {
			if item, ok := a.deploymentList.SelectedItem().(deploymentItem); ok {
				return a, a.scaleDialog.Show(item.deployment.Name, item.deployment.Replicas)
			}
		}
		if a.viewState == ViewDeploymentDetails && a.deploymentDetails.Deployment() != nil {
			dep := a.deploymentDetails.Deployment()
			return a, a.scaleDialog.Show(dep.Name, dep.Replicas)
		}

	case "w":
		// Toggle warnings filter in events view
		if a.viewState == ViewEvents {
			a.eventViewer.ToggleWarningsOnly()
			return a, nil
		}

	case "k":
		// Cycle kind filter in events view
		if a.viewState == ViewEvents {
			a.eventViewer.CycleKindFilter()
			return a, nil
		}

	case "n":
		// Next search match
		if a.viewState == ViewLogs && a.logViewer.SearchQuery() != "" {
			a.searchInput.NextMatch()
			return a, nil
		}
		if a.viewState == ViewCrictlLogs && a.crictlLogViewer.searchQuery != "" {
			a.searchInput.NextMatch()
			return a, nil
		}

	case "N":
		// Previous search match
		if a.viewState == ViewLogs && a.logViewer.SearchQuery() != "" {
			a.searchInput.PrevMatch()
			return a, nil
		}
		if a.viewState == ViewCrictlLogs && a.crictlLogViewer.searchQuery != "" {
			a.searchInput.PrevMatch()
			return a, nil
		}

	case "1":
		// Go to namespaces view
		if a.connectionStatus == domain.StatusConnected && a.viewState != ViewNamespaces {
			a.viewState = ViewNamespaces
			a.loading = true
			return a, a.fetchNamespaces()
		}

	case "2":
		// Go to pods view
		if a.connectionStatus == domain.StatusConnected && a.viewState != ViewPods {
			a.viewState = ViewPods
			a.loading = true
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		}

	case "3":
		// Go to deployments view
		if a.connectionStatus == domain.StatusConnected && a.viewState != ViewDeployments {
			a.viewState = ViewDeployments
			a.loading = true
			return a, a.fetchDeployments()
		}

	case "4":
		// Go to services view
		if a.connectionStatus == domain.StatusConnected && a.viewState != ViewServices {
			a.viewState = ViewServices
			a.loading = true
			return a, a.fetchServices()
		}

	case "5":
		// Go to events view
		if a.connectionStatus == domain.StatusConnected && a.viewState != ViewEvents {
			a.viewState = ViewEvents
			a.loading = true
			return a, tea.Batch(a.fetchEvents(), a.scheduleEventRefresh())
		}

	case "9":
		// Go to SSH hosts view
		if len(a.config.SSHHosts) > 0 {
			a.viewState = ViewSSHHosts
			a.err = nil
			return a, nil
		}

	case "esc":
		switch a.viewState {
		case ViewMain:
			// Go back to pods
			if a.connectionStatus == domain.StatusConnected {
				a.viewState = ViewPods
				return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
			}
			// Or go back to kubeconfig selection
			if len(a.config.KubeConfigs) > 1 {
				a.viewState = ViewKubeConfigSelect
				a.k8sClient = nil
				a.clusterInfo = nil
				a.connectionStatus = domain.StatusDisconnected
				return a, nil
			}
		case ViewLogs:
			// Stop streaming and go back to where we came from
			a.stopLogStream()
			a.logViewer.Clear()
			if a.logSourceView == ViewPods {
				// Came from pods list - go back to pods
				a.viewState = ViewPods
				a.selectedPodName = ""
				return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
			}
			// Came from pod details - go back to pod details
			a.viewState = ViewPodDetails
			a.loading = true
			return a, a.fetchPodDetails(a.selectedPodName)
		case ViewMultiPodLogs:
			a.stopMultiPodStreams()
			a.multiPodLogViewer.Clear()
			a.viewState = ViewPods
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		case ViewPodDetails:
			// Go back to pods
			a.viewState = ViewPods
			a.selectedPodName = ""
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		case ViewPods:
			// Go back to namespaces
			a.viewState = ViewNamespaces
			return a, a.fetchNamespaces()
		case ViewNamespaces:
			// Go back to kubeconfig selection if multiple configs
			if len(a.config.KubeConfigs) > 1 {
				a.viewState = ViewKubeConfigSelect
				a.k8sClient = nil
				a.clusterInfo = nil
				a.connectionStatus = domain.StatusDisconnected
				return a, nil
			}
		case ViewSSHHosts:
			// Go back to pods view (or namespaces if not connected)
			if a.connectionStatus == domain.StatusConnected {
				a.viewState = ViewPods
				return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
			}
			a.viewState = ViewNamespaces
			return a, a.fetchNamespaces()
		case ViewCrictlContainers, ViewNodeInfo:
			// Go back to SSH hosts
			a.closeSSHConnection()
			a.viewState = ViewSSHHosts
			return a, nil
		case ViewCrictlLogs:
			// Stop streaming and go back to containers
			a.stopCrictlLogStream()
			a.crictlLogViewer.Clear()
			a.selectedCrictlContainer = nil
			a.viewState = ViewCrictlContainers
			return a, nil
		case ViewDeployments:
			// Go back to pods
			a.viewState = ViewPods
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		case ViewDeploymentDetails:
			// Go back to deployments
			a.viewState = ViewDeployments
			a.selectedDeployName = ""
			return a, a.fetchDeployments()
		case ViewServices:
			// Go back to pods
			a.viewState = ViewPods
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		case ViewServiceDetails:
			// Go back to services
			a.viewState = ViewServices
			a.selectedServiceName = ""
			return a, a.fetchServices()
		case ViewEvents:
			// Go back to pods
			a.viewState = ViewPods
			return a, tea.Batch(a.fetchPods(), a.schedulePodRefresh())
		}
	}

	// Pass key to child components
	switch a.viewState {
	case ViewKubeConfigSelect:
		var cmd tea.Cmd
		a.kubeConfigList, cmd = a.kubeConfigList.Update(msg)
		return a, cmd
	case ViewNamespaces:
		var cmd tea.Cmd
		a.namespaceList, cmd = a.namespaceList.Update(msg)
		return a, cmd
	case ViewPods:
		var cmd tea.Cmd
		a.podList, cmd = a.podList.Update(msg)
		return a, cmd
	case ViewPodDetails:
		var cmd tea.Cmd
		a.podDetails, cmd = a.podDetails.Update(msg)
		return a, cmd
	case ViewLogs:
		var cmd tea.Cmd
		a.logViewer, cmd = a.logViewer.Update(msg)
		return a, cmd
	case ViewMultiPodLogs:
		var cmd tea.Cmd
		a.multiPodLogViewer, cmd = a.multiPodLogViewer.Update(msg)
		return a, cmd
	case ViewSSHHosts:
		var cmd tea.Cmd
		a.sshHostList, cmd = a.sshHostList.Update(msg)
		return a, cmd
	case ViewCrictlContainers:
		var cmd tea.Cmd
		a.crictlContainerList, cmd = a.crictlContainerList.Update(msg)
		return a, cmd
	case ViewCrictlLogs:
		var cmd tea.Cmd
		a.crictlLogViewer, cmd = a.crictlLogViewer.Update(msg)
		return a, cmd
	case ViewDeployments:
		var cmd tea.Cmd
		a.deploymentList, cmd = a.deploymentList.Update(msg)
		return a, cmd
	case ViewDeploymentDetails:
		var cmd tea.Cmd
		a.deploymentDetails, cmd = a.deploymentDetails.Update(msg)
		return a, cmd
	case ViewServices:
		var cmd tea.Cmd
		a.serviceList, cmd = a.serviceList.Update(msg)
		return a, cmd
	case ViewServiceDetails:
		var cmd tea.Cmd
		a.serviceDetails, cmd = a.serviceDetails.Update(msg)
		return a, cmd
	case ViewEvents:
		var cmd tea.Cmd
		a.eventViewer, cmd = a.eventViewer.Update(msg)
		return a, cmd
	}

	return a, nil
}

// View implements tea.Model
func (a *App) View() string {
	if !a.ready {
		return "Loading..."
	}

	var view string
	switch a.viewState {
	case ViewKubeConfigSelect:
		view = a.renderKubeConfigSelect()
	case ViewConnecting:
		view = a.renderConnecting()
	case ViewNamespaces:
		view = a.renderNamespacesView()
	case ViewPods:
		view = a.renderPodsView()
	case ViewPodDetails:
		view = a.renderPodDetailsView()
	case ViewLogs:
		view = a.renderLogsView()
	case ViewMain:
		view = a.renderMainView()
	case ViewSSHHosts:
		view = a.renderSSHHostsView()
	case ViewSSHConnecting:
		view = a.renderSSHConnecting()
	case ViewCrictlContainers:
		view = a.renderCrictlContainersView()
	case ViewCrictlLogs:
		view = a.renderCrictlLogsView()
	case ViewDeployments:
		view = a.renderDeploymentsView()
	case ViewDeploymentDetails:
		view = a.renderDeploymentDetailsView()
	case ViewServices:
		view = a.renderServicesView()
	case ViewServiceDetails:
		view = a.renderServiceDetailsView()
	case ViewEvents:
		view = a.renderEventsView()
	case ViewMultiPodLogs:
		view = a.renderMultiPodLogsView()
	default:
		view = ""
	}

	// Overlay scale dialog if visible
	if a.scaleDialog.IsVisible() {
		view = a.overlayScaleDialog(view)
	}

	// Overlay help screen if visible
	if a.helpScreen.IsVisible() {
		return a.overlayHelpScreen(view)
	}

	return view
}

// overlayHelpScreen overlays the help screen centered on the view
func (a *App) overlayHelpScreen(view string) string {
	if !a.helpScreen.IsVisible() {
		return view
	}
	return a.placeOverlay(view, a.helpScreen.View())
}

func (a *App) renderKubeConfigSelect() string {
	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(a.kubeConfigList.View())
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderConnecting() string {
	spinnerView := a.spinner.View()
	connectingText := fmt.Sprintf("%s Connecting to cluster...", spinnerView)
	if a.selectedConfig != nil {
		connectingText = fmt.Sprintf("%s Connecting to %s...", spinnerView, a.selectedConfig.Name)
	}

	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(connectingText)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderNamespacesView() string {
	var contentStr string
	if a.loading && a.namespaceCount == 0 {
		contentStr = fmt.Sprintf("%s Loading namespaces...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("Namespaces (%d)", a.namespaceCount)
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		sep := a.renderSeparator()
		headerLine := lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("  %-45s %-12s %s", "NAME", "STATUS", "AGE"))
		contentStr = titleLine + "\n" + sep + "\n" + headerLine + "\n" + a.namespaceList.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderPodsView() string {
	var contentStr string
	if a.loading && a.podCount == 0 {
		contentStr = fmt.Sprintf("%s Loading pods...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("Pods (%d)", a.podCount)
		if a.metricsEnabled {
			title += " [metrics]"
		}
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		sep := a.renderSeparator()

		var headerLine string
		if a.metricsEnabled {
			headerLine = lipgloss.NewStyle().Foreground(colorMuted).
				Render(fmt.Sprintf("  %-45s %-7s %-12s %-8s %-8s %-10s %s", "NAME", "READY", "STATUS", "RESTARTS", "CPU", "MEMORY", "AGE"))
		} else {
			headerLine = lipgloss.NewStyle().Foreground(colorMuted).
				Render(fmt.Sprintf("  %-45s %-7s %-12s %-8s %s", "NAME", "READY", "STATUS", "RESTARTS", "AGE"))
		}
		contentStr = titleLine + "\n" + sep + "\n" + headerLine + "\n" + a.podList.View()
	}

	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	var view string
	view = a.assembleView(content, footer)

	if a.confirmDialog.IsVisible() {
		view = a.overlayDialog(view)
	}
	if a.podMultiSelector.IsVisible() {
		view = a.overlayPodMultiSelector(view)
	}
	return view
}

func (a *App) renderPodDetailsView() string {
	var contentStr string
	if a.loading {
		contentStr = fmt.Sprintf("%s Loading pod details...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("Pod: %s", a.selectedPodName)
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		scrollInfo := lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf(" (%.0f%%)", a.podDetails.ScrollPercent()*100))
		contentStr = titleLine + scrollInfo + "\n" + a.podDetails.View()
	}

	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	var view string
	view = a.assembleView(content, footer)
	if a.confirmDialog.IsVisible() {
		view = a.overlayDialog(view)
	}
	return view
}

func (a *App) renderLogsView() string {
	var contentStr string
	if a.loading {
		contentStr = fmt.Sprintf("%s Loading logs...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		logHeader := a.logViewer.RenderHeader()
		if a.searchInput.IsVisible() {
			logHeader += "\n" + a.searchInput.View()
		}
		contentStr = logHeader + "\n" + a.logViewer.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	var view string
	view = a.assembleView(content, footer)
	if a.containerSelector.IsVisible() {
		view = a.overlayContainerSelector(view)
	}
	return view
}

func (a *App) renderMainView() string {
	content := a.renderContent()
	footer := a.renderFooter()
	return a.assembleView(content, footer)
}

func (a *App) renderHeader() string {
	title := a.styles.Title.Render("k4s")

	// Compact fallback header for narrow terminals
	parts := []string{title}

	if a.clusterInfo != nil && a.connectionStatus == domain.StatusConnected {
		ctx := lipgloss.NewStyle().Foreground(colorMuted).Render(a.clusterInfo.Context)
		ns := lipgloss.NewStyle().Foreground(colorMuted).Render(a.clusterInfo.Namespace)
		parts = append(parts, ctx, ns)
	} else if a.selectedConfig != nil {
		cfg := lipgloss.NewStyle().Foreground(colorMuted).Render(a.selectedConfig.Name)
		parts = append(parts, cfg)
	}

	sep := lipgloss.NewStyle().Foreground(colorDim).Render(" · ")
	headerContent := ""
	for i, p := range parts {
		if i > 0 {
			headerContent += sep
		}
		headerContent += p
	}

	return a.styles.Header.
		Width(a.contentWidth).
		Render(headerContent)
}

func (a *App) renderContent() string {
	if a.err != nil {
		return a.renderError()
	}

	if a.selectedConfig == nil {
		return a.styles.Content.Render("No kubeconfig selected")
	}

	var content string
	if a.clusterInfo != nil && a.connectionStatus == domain.StatusConnected {
		content = fmt.Sprintf(`
  Cluster:    %s
  Context:    %s
  Namespace:  %s
  Status:     Connected

Navigation:
  0 - Namespaces view
  1 - Pods view

Upcoming features:
  • Pod operations (Step 7)
  • Real-time streaming logs (Step 8)
`, a.clusterInfo.Name, a.clusterInfo.Context, a.clusterInfo.Namespace)
	} else {
		content = fmt.Sprintf(`
  Kubeconfig: %s
  Path:       %s
  Status:     %s

Press 'r' to retry connection
`, a.selectedConfig.Name, a.selectedConfig.Path, a.connectionStatus)
	}

	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	return a.styles.Content.
		Width(a.contentWidth).
		Height(h).
		Render(content)
}

// renderSeparator renders a thin horizontal line
func (a *App) renderSeparator() string {
	return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", a.contentWidth))
}

// assembleView combines content and footer, adding sidebar or header as needed
func (a *App) assembleView(content, footer string) string {
	if a.showSidebar {
		sidebar := a.renderSidebar()
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, content, sidebar)
		return lipgloss.JoinVertical(lipgloss.Left, topRow, footer)
	}
	header := a.renderHeader()
	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (a *App) renderFooter() string {
	// Show notification if visible
	if a.notification.IsVisible() {
		return a.styles.Footer.Width(a.width - 4).Render(a.notification.View())
	}

	// Crush-style help: bold key + subtle action, separated by " · "
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	actionStyle := lipgloss.NewStyle().Foreground(colorSubtle)
	sep := lipgloss.NewStyle().Foreground(colorDim).Render(" · ")

	renderHelp := func(pairs ...string) string {
		var parts []string
		for i := 0; i+1 < len(pairs); i += 2 {
			parts = append(parts, keyStyle.Render(pairs[i])+" "+actionStyle.Render(pairs[i+1]))
		}
		result := ""
		for i, p := range parts {
			if i > 0 {
				result += sep
			}
			result += p
		}
		return result
	}

	var helpText string
	switch a.viewState {
	case ViewKubeConfigSelect:
		helpText = renderHelp("↑/↓", "navigate", "enter", "select", "/", "filter", "q", "quit")
	case ViewConnecting:
		helpText = renderHelp("ctrl+c", "cancel")
	case ViewNamespaces:
		helpText = renderHelp("↑/↓", "navigate", "enter", "select", "/", "filter", "2", "pods", "r", "refresh", "esc", "back", "q", "quit")
	case ViewPods:
		helpText = renderHelp("↑/↓", "navigate", "enter", "details", "l", "logs", "L", "multi-log", "d", "delete", "R", "restart", "/", "filter", "r", "refresh", "esc", "back", "q", "quit")
	case ViewPodDetails:
		helpText = renderHelp("↑/↓", "scroll", "l", "logs", "d", "delete", "R", "restart", "r", "refresh", "esc", "back", "q", "quit")
	case ViewLogs:
		helpText = renderHelp("↑/↓", "scroll", "f", "follow", "t", "timestamps", "r", "refresh", "esc", "back", "q", "quit")
	case ViewMain:
		helpText = renderHelp("1", "namespaces", "2", "pods", "r", "retry", "q", "quit")
	case ViewSSHHosts:
		helpText = renderHelp("↑/↓", "navigate", "enter", "connect", "/", "filter", "esc", "back", "q", "quit")
	case ViewSSHConnecting:
		helpText = renderHelp("ctrl+c", "cancel")
	case ViewCrictlContainers:
		helpText = renderHelp("↑/↓", "navigate", "enter", "logs", "/", "filter", "r", "refresh", "esc", "back", "q", "quit")
	case ViewCrictlLogs:
		helpText = renderHelp("↑/↓", "scroll", "f", "follow", "t", "timestamps", "r", "refresh", "esc", "back", "q", "quit")
	case ViewDeployments:
		helpText = renderHelp("↑/↓", "navigate", "enter", "details", "s", "scale", "R", "restart", "d", "delete", "/", "filter", "r", "refresh", "esc", "back", "q", "quit")
	case ViewDeploymentDetails:
		helpText = renderHelp("↑/↓", "scroll", "s", "scale", "R", "restart", "d", "delete", "r", "refresh", "esc", "back", "q", "quit")
	case ViewServices:
		helpText = renderHelp("↑/↓", "navigate", "enter", "details", "/", "filter", "r", "refresh", "esc", "back", "q", "quit")
	case ViewServiceDetails:
		helpText = renderHelp("↑/↓", "scroll", "r", "refresh", "esc", "back", "q", "quit")
	case ViewEvents:
		helpText = renderHelp("↑/↓", "scroll", "f", "follow", "w", "warnings", "k", "kind", "r", "refresh", "esc", "back", "q", "quit")
	case ViewMultiPodLogs:
		helpText = renderHelp("↑/↓", "scroll", "f", "follow", "esc", "back", "q", "quit")
	}

	// Thin separator above help
	sepLine := lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", a.width-4))

	// When sidebar is hidden, also show status badge
	if !a.showSidebar && a.connectionStatus == domain.StatusConnected && a.clusterInfo != nil {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(colorSuccess).Padding(0, 1)
		statusBadge := statusStyle.Render(fmt.Sprintf("%s · %s", a.clusterInfo.Context, a.clusterInfo.Namespace))
		return sepLine + "\n" + a.styles.Footer.Width(a.width-4).Render(helpText+"  "+statusBadge)
	}

	return sepLine + "\n" + a.styles.Footer.Width(a.width-4).Render(helpText)
}

func (a *App) renderError() string {
	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	return a.styles.Content.
		Width(a.contentWidth).
		Height(h).
		Render(renderErrorBox(a.err, a.contentWidth))
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// placeOverlay places a dialog on top of an existing view, preserving background content
func (a *App) placeOverlay(background, overlay string) string {
	bgLines := strings.Split(background, "\n")
	ovLines := strings.Split(overlay, "\n")

	ovWidth := lipgloss.Width(overlay)
	ovHeight := len(ovLines)

	// Center the overlay
	startX := (a.width - ovWidth) / 2
	startY := (a.height - ovHeight) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	// Ensure background has enough lines
	for len(bgLines) < a.height {
		bgLines = append(bgLines, "")
	}

	// Overlay each line
	for i, ovLine := range ovLines {
		bgIdx := startY + i
		if bgIdx >= len(bgLines) {
			break
		}

		bgLine := bgLines[bgIdx]
		// Pad background line to startX if needed
		bgVisWidth := lipgloss.Width(bgLine)
		if bgVisWidth < startX {
			bgLine += strings.Repeat(" ", startX-bgVisWidth)
		}

		// Build: background prefix + overlay line + background suffix
		// We use ANSI-aware splitting via lipgloss
		prefix := ansiTruncate(bgLine, startX)
		suffix := ""
		endX := startX + ovWidth
		if bgVisWidth > endX {
			suffix = ansiCutLeft(bgLine, endX)
		}

		bgLines[bgIdx] = prefix + ovLine + suffix
	}

	// Trim to screen height
	if len(bgLines) > a.height {
		bgLines = bgLines[:a.height]
	}

	return strings.Join(bgLines, "\n")
}

// ansiTruncate truncates a string to n visible characters, preserving ANSI codes
func ansiTruncate(s string, n int) string {
	var result strings.Builder
	visible := 0
	i := 0
	for i < len(s) && visible < n {
		if s[i] == '\x1b' {
			match := ansiRegex.FindStringIndex(s[i:])
			if match != nil && match[0] == 0 {
				result.WriteString(s[i : i+match[1]])
				i += match[1]
				continue
			}
		}
		result.WriteByte(s[i])
		visible++
		i++
	}
	// Pad if needed
	for visible < n {
		result.WriteByte(' ')
		visible++
	}
	return result.String()
}

// ansiCutLeft returns the portion of s after n visible characters
func ansiCutLeft(s string, n int) string {
	visible := 0
	i := 0
	for i < len(s) && visible < n {
		if s[i] == '\x1b' {
			match := ansiRegex.FindStringIndex(s[i:])
			if match != nil && match[0] == 0 {
				i += match[1]
				continue
			}
		}
		visible++
		i++
	}
	if i >= len(s) {
		return ""
	}
	return s[i:]
}

// overlayContainerSelector overlays the container selector centered on screen
func (a *App) overlayContainerSelector(view string) string {
	selector := a.containerSelector.View()
	if selector == "" {
		return view
	}
	return a.placeOverlay(view, selector)
}

// overlayDialog overlays the confirmation dialog centered on screen
func (a *App) overlayDialog(view string) string {
	dialog := a.confirmDialog.View()
	if dialog == "" {
		return view
	}
	return a.placeOverlay(view, dialog)
}

// overlayScaleDialog overlays the scale dialog centered on screen
func (a *App) overlayScaleDialog(view string) string {
	dialog := a.scaleDialog.View()
	if dialog == "" {
		return view
	}
	return a.placeOverlay(view, dialog)
}

// Deployments view
func (a *App) renderDeploymentsView() string {
	var contentStr string
	if a.loading && a.deploymentCount == 0 {
		contentStr = fmt.Sprintf("%s Loading deployments...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("Deployments (%d)", a.deploymentCount)
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		sep := a.renderSeparator()
		headerLine := lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("  %-40s %-10s %-10s %-10s %s", "NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"))
		contentStr = titleLine + "\n" + sep + "\n" + headerLine + "\n" + a.deploymentList.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	var view string
	view = a.assembleView(content, footer)
	if a.confirmDialog.IsVisible() {
		view = a.overlayDialog(view)
	}
	if a.scaleDialog.IsVisible() {
		view = a.overlayScaleDialog(view)
	}
	return view
}

func (a *App) renderDeploymentDetailsView() string {
	var contentStr string
	if a.loading {
		contentStr = fmt.Sprintf("%s Loading deployment details...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		contentStr = a.deploymentDetails.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

// Services view
func (a *App) renderServicesView() string {
	var contentStr string
	if a.loading && a.serviceCount == 0 {
		contentStr = fmt.Sprintf("%s Loading services...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("Services (%d)", a.serviceCount)
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		sep := a.renderSeparator()
		headerLine := lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("  %-30s %-14s %-16s %-20s %-20s %s", "NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORTS", "AGE"))
		contentStr = titleLine + "\n" + sep + "\n" + headerLine + "\n" + a.serviceList.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderServiceDetailsView() string {
	var contentStr string
	if a.loading {
		contentStr = fmt.Sprintf("%s Loading service details...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		contentStr = a.serviceDetails.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

// Events view
func (a *App) renderEventsView() string {
	var contentStr string
	if a.loading && a.eventViewer.TotalEvents() == 0 {
		contentStr = fmt.Sprintf("%s Loading events...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		titleLine := a.eventViewer.RenderHeader()
		contentStr = titleLine + "\n" + a.eventViewer.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

// SSH-related render functions

func (a *App) renderSSHHostsView() string {
	var contentStr string
	if len(a.config.SSHHosts) == 0 {
		contentStr = "No SSH hosts configured.\n\nAdd hosts to ~/.k4s/config.yaml:\n\nssh_hosts:\n  - name: \"my-node\"\n    host: \"192.168.1.100\"\n    user: \"admin\"\n    key_path: \"~/.ssh/id_rsa\"\n    port: 22"
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		title := fmt.Sprintf("SSH Hosts (%d)", len(a.config.SSHHosts))
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(title)
		sep := a.renderSeparator()
		contentStr = titleLine + "\n" + sep + "\n" + a.sshHostList.View()
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderSSHConnecting() string {
	spinnerView := a.spinner.View()
	var connectingText string
	if a.selectedSSHHost != nil {
		connectingText = fmt.Sprintf("%s Connecting to %s@%s...", spinnerView, a.selectedSSHHost.User, a.selectedSSHHost.Host)
	} else {
		connectingText = fmt.Sprintf("%s Connecting via SSH...", spinnerView)
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(connectingText)
	footer := a.renderFooter()

	var view string
	view = a.assembleView(content, footer)

	// Overlay passphrase input if visible
	if a.passphraseInput.IsVisible() {
		dialog := a.passphraseInput.View()
		return lipgloss.Place(
			a.width,
			a.height,
			lipgloss.Center,
			lipgloss.Center,
			dialog,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("#1a1a1a")),
		)
	}

	return view
}

func (a *App) renderCrictlContainersView() string {
	var contentStr string
	if a.loading && len(a.crictlContainers) == 0 {
		contentStr = fmt.Sprintf("%s Loading containers...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		var titleParts []string
		if a.selectedSSHHost != nil {
			titleParts = append(titleParts, fmt.Sprintf("Node: %s", a.selectedSSHHost.Name))
		}
		titleParts = append(titleParts, fmt.Sprintf("Containers: %d", len(a.crictlContainers)))
		titleLine := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).
			Render(joinStrings(titleParts, " · "))
		sep := a.renderSeparator()

		var nodeInfoLine string
		if a.nodeInfo != nil {
			infoStyle := lipgloss.NewStyle().Foreground(colorMuted)
			infoParts := []string{}
			if a.nodeInfo.Hostname != "" {
				infoParts = append(infoParts, a.nodeInfo.Hostname)
			}
			if a.nodeInfo.OS != "" {
				infoParts = append(infoParts, a.nodeInfo.OS)
			}
			if a.nodeInfo.Memory != "" {
				infoParts = append(infoParts, fmt.Sprintf("Mem: %s", a.nodeInfo.Memory))
			}
			if a.nodeInfo.LoadAvg != "" {
				infoParts = append(infoParts, fmt.Sprintf("Load: %s", a.nodeInfo.LoadAvg))
			}
			nodeInfoLine = infoStyle.Render(joinStrings(infoParts, " | "))
		}

		headerLine := lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("  %-25s %-30s %-15s %-10s %s", "NAME", "POD", "NAMESPACE", "STATE", "AGE"))

		if nodeInfoLine != "" {
			contentStr = titleLine + "\n" + nodeInfoLine + "\n" + sep + "\n" + headerLine + "\n" + a.crictlContainerList.View()
		} else {
			contentStr = titleLine + "\n" + sep + "\n" + headerLine + "\n" + a.crictlContainerList.View()
		}
	}

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

func (a *App) renderCrictlLogsView() string {
	var contentStr string
	if a.loading {
		contentStr = fmt.Sprintf("%s Loading logs...", a.spinner.View())
	} else if a.err != nil {
		contentStr = a.renderError()
	} else {
		logHeader := a.crictlLogViewer.RenderHeader()
		if a.searchInput.IsVisible() {
			logHeader += "\n" + a.searchInput.View()
		}
		contentStr = logHeader + "\n" + a.crictlLogViewer.View()
	}

	h := a.height - 10
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

// renderMultiPodLogsView renders the multi-pod log view
func (a *App) renderMultiPodLogsView() string {
	logHeader := a.multiPodLogViewer.RenderHeader()
	contentStr := logHeader + "\n" + a.multiPodLogViewer.View()

	h := a.height - 12
	if a.showSidebar {
		h = a.height - 4
	}
	content := a.styles.Content.Width(a.contentWidth).Height(h).Render(contentStr)
	footer := a.renderFooter()

	return a.assembleView(content, footer)
}

// overlayPodMultiSelector overlays the pod multi-selector centered on screen
func (a *App) overlayPodMultiSelector(view string) string {
	selector := a.podMultiSelector.View()
	if selector == "" {
		return view
	}
	return a.placeOverlay(view, selector)
}

// startMultiPodStreaming starts log streaming for multiple pods simultaneously
func (a *App) startMultiPodStreaming(podNames []string) tea.Cmd {
	if a.k8sClient == nil {
		return nil
	}

	a.stopMultiPodStreams()

	// Build a map from pod name to first container
	podContainerMap := make(map[string]string)
	for _, item := range a.podList.Items() {
		if pi, ok := item.(podItem); ok {
			for _, name := range podNames {
				if pi.pod.Name == name && len(pi.pod.Containers) > 0 {
					podContainerMap[name] = pi.pod.Containers[0].Name
				}
			}
		}
	}

	a.multiPodLogViewer.SetPods(podNames)
	a.multiPodLogViewer.SetFollowing(true)
	a.viewState = ViewMultiPodLogs

	ctx, cancel := context.WithCancel(context.Background())
	a.multiPodStreamCancel = cancel
	a.multiPodStreamCtx = ctx
	a.multiPodStreamActive = true
	a.multiPodActiveStreams = len(podNames)
	a.multiPodLineChanMap = make(map[string]<-chan string)
	a.multiPodContainerMap = make(map[string]string)

	namespace := a.k8sClient.CurrentNamespace()
	var cmds []tea.Cmd

	for _, podName := range podNames {
		container := podContainerMap[podName]
		a.multiPodContainerMap[podName] = container

		cmds = append(cmds, a.startSinglePodStream(ctx, namespace, podName, container, true))
	}

	return tea.Batch(cmds...)
}

// startSinglePodStream starts a log stream for a single pod within the multi-pod context
func (a *App) startSinglePodStream(ctx context.Context, namespace, podName, container string, withHistory bool) tea.Cmd {
	lineChan := make(chan string, 100)
	a.multiPodLineChanMap[podName] = lineChan

	var tailLines int64
	if withHistory {
		tailLines = 100
	}

	pn := podName
	cn := container

	go func() {
		defer close(lineChan)
		_ = a.k8sClient.StreamPodLogs(ctx, namespace, pn, k8s.LogOptions{
			Container:  cn,
			TailLines:  tailLines,
			Timestamps: false,
			Follow:     true,
		}, lineChan)
	}()

	return a.waitForMultiPodLogLine(pn, cn, lineChan)
}

// waitForMultiPodLogLine returns a command that reads from a pod's log channel
func (a *App) waitForMultiPodLogLine(podName, container string, ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return multiPodLogStreamEndedMsg{podName: podName}
		}
		return multiPodLogLineMsg{podName: podName, container: container, line: line}
	}
}

// handleMultiPodLogLine handles a log line from a multi-pod stream
func (a *App) handleMultiPodLogLine(msg multiPodLogLineMsg) (tea.Model, tea.Cmd) {
	if a.viewState != ViewMultiPodLogs {
		return a, nil
	}

	a.multiPodLogViewer.AppendEntry(MultiPodLogEntry{
		PodName:   msg.podName,
		Container: msg.container,
		Line:      msg.line,
	})

	if ch, ok := a.multiPodLineChanMap[msg.podName]; ok {
		return a, a.waitForMultiPodLogLine(msg.podName, msg.container, ch)
	}
	return a, nil
}

// handleMultiPodLogStreamEnded handles the end of a single pod's log stream
func (a *App) handleMultiPodLogStreamEnded(msg multiPodLogStreamEndedMsg) (tea.Model, tea.Cmd) {
	delete(a.multiPodLineChanMap, msg.podName)

	// If context was cancelled (user stopped), don't restart
	if msg.err == context.Canceled {
		a.multiPodActiveStreams--
		return a, nil
	}

	// Auto-restart the stream if still in multi-pod log view and following
	if a.viewState == ViewMultiPodLogs && a.multiPodStreamActive && a.multiPodStreamCtx != nil {
		container := a.multiPodContainerMap[msg.podName]
		namespace := a.k8sClient.CurrentNamespace()
		logger.Debug("Multi-pod stream ended, auto-restarting", "pod", msg.podName)
		// Restart with TailLines=0 (only new logs, since we already have history)
		cmd := a.startSinglePodStream(a.multiPodStreamCtx, namespace, msg.podName, container, false)
		return a, cmd
	}

	a.multiPodActiveStreams--
	if a.multiPodActiveStreams <= 0 {
		a.multiPodStreamActive = false
	}
	return a, nil
}

// stopMultiPodStreams stops all multi-pod log streams
func (a *App) stopMultiPodStreams() {
	if a.multiPodStreamCancel != nil {
		a.multiPodStreamCancel()
		a.multiPodStreamCancel = nil
	}
	a.multiPodStreamCtx = nil
	a.multiPodStreamActive = false
	a.multiPodActiveStreams = 0
	a.multiPodLineChanMap = nil
	a.multiPodContainerMap = nil
}
