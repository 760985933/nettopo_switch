export namespace main {
	
	export class Profile {
	    id: string;
	    name: string;
	    provider: string;
	    baseURL: string;
	    apiKey: string;
	    defaultModel: string;
	    mappings: Record<string, string>;
	    apiType?: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.baseURL = source["baseURL"];
	        this.apiKey = source["apiKey"];
	        this.defaultModel = source["defaultModel"];
	        this.mappings = source["mappings"];
	        this.apiType = source["apiType"];
	    }
	}
	export class InstanceConfig {
	    listenHost: string;
	    listenPort: number;
	    requestTimeoutMs: number;
	    maxRetries: number;
	    mappings: Record<string, string>;
	    headers: Record<string, string>;
	    currentProfileId: string;
	    proxyProfileIds?: string[];
	
	    static createFrom(source: any = {}) {
	        return new InstanceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listenHost = source["listenHost"];
	        this.listenPort = source["listenPort"];
	        this.requestTimeoutMs = source["requestTimeoutMs"];
	        this.maxRetries = source["maxRetries"];
	        this.mappings = source["mappings"];
	        this.headers = source["headers"];
	        this.currentProfileId = source["currentProfileId"];
	        this.proxyProfileIds = source["proxyProfileIds"];
	    }
	}
	export class AppConfig {
	    enableAutoStart: boolean;
	    minimizeToTray: boolean;
	    logRetentionDays: number;
	    compactMode: boolean;
	    pluginUnlockEnabled: boolean;
	    listenHost: string;
	    listenPort: number;
	    deepseekBaseURL: string;
	    apiKey: string;
	    defaultModel: string;
	    requestTimeoutMs: number;
	    maxRetries: number;
	    mappings: Record<string, string>;
	    headers: Record<string, string>;
	    currentProfileId?: string;
	    proxyProfileIds?: string[];
	    instances?: Record<string, InstanceConfig>;
	    profiles?: Record<string, Profile>;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enableAutoStart = source["enableAutoStart"];
	        this.minimizeToTray = source["minimizeToTray"];
	        this.logRetentionDays = source["logRetentionDays"];
	        this.compactMode = source["compactMode"];
	        this.pluginUnlockEnabled = source["pluginUnlockEnabled"];
	        this.listenHost = source["listenHost"];
	        this.listenPort = source["listenPort"];
	        this.deepseekBaseURL = source["deepseekBaseURL"];
	        this.apiKey = source["apiKey"];
	        this.defaultModel = source["defaultModel"];
	        this.requestTimeoutMs = source["requestTimeoutMs"];
	        this.maxRetries = source["maxRetries"];
	        this.mappings = source["mappings"];
	        this.headers = source["headers"];
	        this.currentProfileId = source["currentProfileId"];
	        this.proxyProfileIds = source["proxyProfileIds"];
	        this.instances = this.convertValues(source["instances"], InstanceConfig, true);
	        this.profiles = this.convertValues(source["profiles"], Profile, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CodexSession {
	    id: string;
	    title: string;
	    model: string;
	    modelProvider: string;
	    messageCount: number;
	    createdAt: string;
	    isArchived: boolean;
	    cwd: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.model = source["model"];
	        this.modelProvider = source["modelProvider"];
	        this.messageCount = source["messageCount"];
	        this.createdAt = source["createdAt"];
	        this.isArchived = source["isArchived"];
	        this.cwd = source["cwd"];
	    }
	}
	export class HealthCheckItem {
	    name: string;
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthCheckItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class HealthCheckResult {
	    ok: boolean;
	    checks: HealthCheckItem[];
	
	    static createFrom(source: any = {}) {
	        return new HealthCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.checks = this.convertValues(source["checks"], HealthCheckItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LogEntry {
	    id: string;
	    level: string;
	    timestamp: string;
	    source: string;
	    message: string;
	    requestId?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.level = source["level"];
	        this.timestamp = source["timestamp"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.requestId = source["requestId"];
	    }
	}
	export class MigrationResult {
	    migratedCount: number;
	    backupPath: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new MigrationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.migratedCount = source["migratedCount"];
	        this.backupPath = source["backupPath"];
	        this.error = source["error"];
	    }
	}
	export class ModelStats {
	    provider: string;
	    model: string;
	    requestCount: number;
	    successCount: number;
	    failureCount: number;
	    totalTokens: number;
	    promptTokens: number;
	    completionTokens: number;
	    avgDurationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.requestCount = source["requestCount"];
	        this.successCount = source["successCount"];
	        this.failureCount = source["failureCount"];
	        this.totalTokens = source["totalTokens"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.avgDurationMs = source["avgDurationMs"];
	    }
	}
	export class ProxyStatusPayload {
	    source: string;
	    status: string;
	    listenAddress: string;
	    startedAt: string;
	    uptimeSeconds: number;
	    lastError: string;
	    requestCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatusPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.status = source["status"];
	        this.listenAddress = source["listenAddress"];
	        this.startedAt = source["startedAt"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	        this.lastError = source["lastError"];
	        this.requestCount = source["requestCount"];
	    }
	}
	export class OverviewSnapshot {
	    config: AppConfig;
	    status: ProxyStatusPayload;
	    recentLogs: LogEntry[];
	    quickTips: string[];
	    defaults: Record<string, string>;
	    features: Record<string, boolean>;
	
	    static createFrom(source: any = {}) {
	        return new OverviewSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], AppConfig);
	        this.status = this.convertValues(source["status"], ProxyStatusPayload);
	        this.recentLogs = this.convertValues(source["recentLogs"], LogEntry);
	        this.quickTips = source["quickTips"];
	        this.defaults = source["defaults"];
	        this.features = source["features"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProjectThreadInfo {
	    root: string;
	    interactiveThreads: number;
	    firstPageThreads: number;
	    exactCwdMatches: number;
	    verbatimCwdRows: number;
	    topRank: number;
	    ranks: number[];
	    rankPreview: string;
	    providerCounts: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new ProjectThreadInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.interactiveThreads = source["interactiveThreads"];
	        this.firstPageThreads = source["firstPageThreads"];
	        this.exactCwdMatches = source["exactCwdMatches"];
	        this.verbatimCwdRows = source["verbatimCwdRows"];
	        this.topRank = source["topRank"];
	        this.ranks = source["ranks"];
	        this.rankPreview = source["rankPreview"];
	        this.providerCounts = source["providerCounts"];
	    }
	}
	
	export class SandboxWorkspaceConfig {
	    networkAccess: boolean;
	    sandboxMode: string;
	    approvalPolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new SandboxWorkspaceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.networkAccess = source["networkAccess"];
	        this.sandboxMode = source["sandboxMode"];
	        this.approvalPolicy = source["approvalPolicy"];
	    }
	}
	export class SessionMessage {
	    role: string;
	    content: string;
	    timestamp: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	    }
	}
	export class SessionDetail {
	    session: CodexSession;
	    messages: SessionMessage[];
	
	    static createFrom(source: any = {}) {
	        return new SessionDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = this.convertValues(source["session"], CodexSession);
	        this.messages = this.convertValues(source["messages"], SessionMessage);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SyncRepairStats {
	    userEventRowsNeedingRepair: number;
	    cwdRowsNeedingRepair: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncRepairStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userEventRowsNeedingRepair = source["userEventRowsNeedingRepair"];
	        this.cwdRowsNeedingRepair = source["cwdRowsNeedingRepair"];
	    }
	}
	export class SyncRolloutInfo {
	    sessions: Record<string, number>;
	    archivedSessions: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new SyncRolloutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = source["sessions"];
	        this.archivedSessions = source["archivedSessions"];
	    }
	}
	export class SyncResult {
	    codexHome: string;
	    targetProvider: string;
	    previousProvider: string;
	    backupDir: string;
	    backupDurationMs: number;
	    changedSessionFiles: number;
	    skippedLockedFiles: string[];
	    sqliteRowsUpdated: number;
	    sqliteProviderRowsUpdated: number;
	    sqliteUserEventRowsUpdated: number;
	    sqliteCwdRowsUpdated: number;
	    updatedWorkspaceRoots: number;
	    savedWorkspaceRootCount: number;
	    sqlitePresent: boolean;
	    rolloutCountsBefore: SyncRolloutInfo;
	    encryptedContentWarning?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codexHome = source["codexHome"];
	        this.targetProvider = source["targetProvider"];
	        this.previousProvider = source["previousProvider"];
	        this.backupDir = source["backupDir"];
	        this.backupDurationMs = source["backupDurationMs"];
	        this.changedSessionFiles = source["changedSessionFiles"];
	        this.skippedLockedFiles = source["skippedLockedFiles"];
	        this.sqliteRowsUpdated = source["sqliteRowsUpdated"];
	        this.sqliteProviderRowsUpdated = source["sqliteProviderRowsUpdated"];
	        this.sqliteUserEventRowsUpdated = source["sqliteUserEventRowsUpdated"];
	        this.sqliteCwdRowsUpdated = source["sqliteCwdRowsUpdated"];
	        this.updatedWorkspaceRoots = source["updatedWorkspaceRoots"];
	        this.savedWorkspaceRootCount = source["savedWorkspaceRootCount"];
	        this.sqlitePresent = source["sqlitePresent"];
	        this.rolloutCountsBefore = this.convertValues(source["rolloutCountsBefore"], SyncRolloutInfo);
	        this.encryptedContentWarning = source["encryptedContentWarning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SyncStatusResult {
	    codexHome: string;
	    currentProvider: string;
	    currentProviderImplicit: boolean;
	    configuredProviders: string[];
	    rolloutCounts: SyncRolloutInfo;
	    lockedRolloutFiles: string[];
	    encryptedContentCounts?: SyncRolloutInfo;
	    encryptedContentWarning?: string;
	    sqliteCounts?: SyncRolloutInfo;
	    sqliteUnreadable: boolean;
	    sqliteError?: string;
	    sqliteRepairStats?: SyncRepairStats;
	    projectThreadVisibility: ProjectThreadInfo[];
	    backupRoot: string;
	    backupCount: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codexHome = source["codexHome"];
	        this.currentProvider = source["currentProvider"];
	        this.currentProviderImplicit = source["currentProviderImplicit"];
	        this.configuredProviders = source["configuredProviders"];
	        this.rolloutCounts = this.convertValues(source["rolloutCounts"], SyncRolloutInfo);
	        this.lockedRolloutFiles = source["lockedRolloutFiles"];
	        this.encryptedContentCounts = this.convertValues(source["encryptedContentCounts"], SyncRolloutInfo);
	        this.encryptedContentWarning = source["encryptedContentWarning"];
	        this.sqliteCounts = this.convertValues(source["sqliteCounts"], SyncRolloutInfo);
	        this.sqliteUnreadable = source["sqliteUnreadable"];
	        this.sqliteError = source["sqliteError"];
	        this.sqliteRepairStats = this.convertValues(source["sqliteRepairStats"], SyncRepairStats);
	        this.projectThreadVisibility = this.convertValues(source["projectThreadVisibility"], ProjectThreadInfo);
	        this.backupRoot = source["backupRoot"];
	        this.backupCount = source["backupCount"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TimeSeriesPoint {
	    date: string;
	    totalTokens: number;
	    promptTokens: number;
	    completionTokens: number;
	    requestCount: number;
	
	    static createFrom(source: any = {}) {
	        return new TimeSeriesPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.totalTokens = source["totalTokens"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.requestCount = source["requestCount"];
	    }
	}
	export class UpdateCheckResult {
	    currentVersion: string;
	    latestVersion: string;
	    hasUpdate: boolean;
	    downloadUrl: string;
	    notes: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.hasUpdate = source["hasUpdate"];
	        this.downloadUrl = source["downloadUrl"];
	        this.notes = source["notes"];
	        this.checkedAt = source["checkedAt"];
	    }
	}
	export class UsageBalance {
	    availableBalance: string;
	    totalBalance: string;
	    currency: string;
	    isDepleted: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UsageBalance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.availableBalance = source["availableBalance"];
	        this.totalBalance = source["totalBalance"];
	        this.currency = source["currency"];
	        this.isDepleted = source["isDepleted"];
	        this.error = source["error"];
	    }
	}
	export class UsageStats {
	    provider: string;
	    requestCount: number;
	    successCount: number;
	    failureCount: number;
	    totalTokens: number;
	    promptTokens: number;
	    completionTokens: number;
	    avgDurationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.requestCount = source["requestCount"];
	        this.successCount = source["successCount"];
	        this.failureCount = source["failureCount"];
	        this.totalTokens = source["totalTokens"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.avgDurationMs = source["avgDurationMs"];
	    }
	}
	export class UsageStatsResponse {
	    today: UsageStats[];
	    thisWeek: UsageStats[];
	    thisMonth: UsageStats[];
	    thisYear: UsageStats[];
	    models: ModelStats[];
	    timeSeries: TimeSeriesPoint[];
	
	    static createFrom(source: any = {}) {
	        return new UsageStatsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.today = this.convertValues(source["today"], UsageStats);
	        this.thisWeek = this.convertValues(source["thisWeek"], UsageStats);
	        this.thisMonth = this.convertValues(source["thisMonth"], UsageStats);
	        this.thisYear = this.convertValues(source["thisYear"], UsageStats);
	        this.models = this.convertValues(source["models"], ModelStats);
	        this.timeSeries = this.convertValues(source["timeSeries"], TimeSeriesPoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

