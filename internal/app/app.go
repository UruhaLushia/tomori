package app

import (
	"context"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launchrequest"
	"github.com/UruhaLushia/tomori/internal/logstore"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	minimumWindowWidth  = 760
	minimumWindowHeight = 540
)

type app struct {
	ctx                 context.Context
	shutdown            context.CancelFunc
	workers             sync.WaitGroup
	operationID         uint64
	manifestID          uint64
	scanID              uint64
	scanCancel          context.CancelFunc
	javaID              uint64
	operations          map[uint64]*operation
	operationOrder      []uint64
	activeOperations    int
	downloadRows        []page.DownloadJob
	downloadClosed      time.Time
	manifestCancel      context.CancelFunc
	games               map[string]*gameSession
	downloadBudget      *download.Budget
	logFile             *logstore.File
	logMu               sync.Mutex
	logQueueMu          sync.Mutex
	logWake             chan struct{}
	logPending          []page.LogLine
	logPendingStart     int
	logScheduled        bool
	logStore            *logstore.Store
	logLimitError       string
	logHistory          logHistoryState
	logRevision         uint64
	win                 *mygo.Window
	zoomWorkArea        mygo.Rectangle
	zoomApplied         int
	zoomPending         *zoomPreview
	zoomOutline         zoomOutlineState
	tray                *mygo.Tray
	trayFailed          bool
	launcherVisibility  config.LauncherVisibility
	waitingForGames     bool
	forceQuit           bool
	recreatingWindow    bool
	fpsWindow           *mygo.Window
	fpsWindowClosing    bool
	fpsMu               sync.Mutex
	fpsFrameStart       time.Time
	fpsFrameCount       int
	fpsWarmup           bool
	fpsSamples          []fpsSample
	fpsAccelerated      bool
	cfg                 *config.Config
	store               *config.Store
	pages               pageState
	home                homeState
	settingsDirectories []config.GameDirectory
	storageChanging     bool
	storageError        string
	pathBusy            bool
	shortcutBusy        bool
	directoryPicker     directoryPicker
	directoryEditor     directoryEditor
	resources           resourceState
	resourceImport      resourceImportState
	instanceContent     instanceContentState
	servers             serverState
	appearance          appearanceState
	dialogs             dialogState
	packBrowser         packBrowserState
	instanceSettings    page.InstanceSettingsData
	instanceEdit        instanceEditState
	instanceCopy        instanceCopyState
	worldEdit           worldEditState
	worldArchive        worldArchiveState
	worldBackups        worldBackupsState
	screenshotEdit      screenshotEditState
	logDiagnostic       logDiagnosticState
	packExport          packExportState
	packUpdate          packUpdateState
	packAssociate       packAssociateState
	packRestore         packRestoreState
	packRecovery        packRecoveryState
	components          componentsState
	loaderCatalog       loaderCatalogState
	icons               instanceIconState
	cacheError          string
	cacheStatus         string
	cacheUsage          cacheUsageState
	authID              uint64
	authCancel          context.CancelFunc
	authBusy            bool
	authCode            string
	authURL             string
	authStatus          string
	accountError        string
	externalAuth        externalAuthState
	accountMu           sync.Mutex
	accountUpdates      []accountUpdate
	avatars             map[string]accountAvatar
	avatarID            uint64

	section       int
	page          int
	pageDirection int
	sectionScroll ui.ScrollState

	downloadCategory      string
	downloadCategoryStrip kit.ScrollTabsState
	downloadVersionQuery  string
	downloadVersionFilter int
	downloadVersionsList  ui.ListState

	versions        []minecraft.VersionSummary
	manifestError   string
	manifestLoading bool

	openSection int

	installed           map[string]bool
	installedList       []minecraft.Instance
	java                []java.Runtime
	javaLoading         bool
	javaDownloading     bool
	javaDownloadOpen    bool
	javaDownloadRequest java.DownloadRequest
	instancesReady      bool
	launchRequests      []launchrequest.Request
	fonts               []string
	fontsLoading        bool
	memory              systeminfo.Memory
	memoryError         string

	logs     []page.LogLine
	logStart int

	downloadOpen    bool
	notices         []ui.Toast
	installSpace    spaceAssessment
	packSpace       spaceAssessment
	resourceSpace   spaceAssessment
	packUpdateSpace spaceAssessment
}

func Run(request launchrequest.Request) error {
	mygo.App.SetName("tomori")
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	store, err := config.Open()
	if err != nil {
		return err
	}
	configureGPU(store.Config.Appearance.GPU)
	if err := os.MkdirAll(store.LogDir(), 0o755); err != nil {
		return err
	}
	logs, err := store.LogStore()
	if err != nil {
		return err
	}
	logFile, err := logs.Open("launcher.log", true)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()
	mygo.App.SetPath(mygo.PathUserData, store.Dir())
	a := &app{
		cfg:            &store.Config,
		store:          store,
		installed:      map[string]bool{},
		downloadBudget: download.NewBudget(store.Config.Download.Workers, max(store.Config.Download.Workers, store.Config.Download.Connections)),
	}
	a.openSection = -1
	a.ctx, a.shutdown = context.WithCancel(context.Background())
	a.logFile = logFile
	a.logStore = logs
	mygo.App.SetLanguages("zh-Hans", "en")
	mygo.App.OnLocaleChanged(func(string) { a.updateTray() })
	mygo.App.OnActivate(func(bool) { a.showLauncher() })
	mygo.App.OnSecondInstance(a.receiveArguments)
	mygo.App.OnOpenURL(a.receiveURL)
	mygo.App.SetLocale(a.cfg.Appearance.Language)
	if !request.URL {
		a.receiveLaunchRequest(request)
	}
	if err := mygo.App.RegisterURLScheme("tomori"); err != nil {
		a.logf(locales.CouldNotRegisterUrlHandler(), err)
	}
	a.settingsDirectories = slices.Clone(a.cfg.Directories.Entries)
	mygo.App.SetWindowStateStore(windowStateStore{app: a})
	mygo.Theme.SetSource(a.appearanceSource())
	mygo.App.WhenReady(func() {
		a.applyAppearance(mygo.Theme.IsDark())
		a.warmup()
		a.createMainWindow()
		a.startLogUpdates()
		a.bootstrap()
	})
	err = mygo.App.Run()
	a.shutdown()
	a.workers.Wait()
	a.applyAccountUpdates()
	a.flushPlayHistory()
	if saveErr := a.store.Save(a.cfg.Clone()); saveErr != nil {
		return saveErr
	}
	return err
}

func (a *app) bootstrap() {
	a.logf("%s", locales.TomoriStarted())
	for _, identity := range a.cfg.Accounts.Entries {
		a.loadAccountAvatar(identity)
	}
	a.scanInstalled()
	a.loadPackRecoveries(true)
	a.rescanJava()
	a.loadMemory()
	a.prepareCache()
	a.reclaimStartupMemory()
}
