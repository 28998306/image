export namespace dto {
	
	export class GptBatchRefreshResp {
	    refreshed: number;
	    failed: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new GptBatchRefreshResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refreshed = source["refreshed"];
	        this.failed = source["failed"];
	        this.errors = source["errors"];
	    }
	}
	export class GptPoolDetailResp {
	    id: number;
	    email: string;
	    access_token: string;
	    refresh_token: string;
	    id_token: string;
	
	    static createFrom(source: any = {}) {
	        return new GptPoolDetailResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.id_token = source["id_token"];
	    }
	}
	export class GptPoolImportResult {
	    imported: number;
	    updated: number;
	    skipped: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new GptPoolImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.updated = source["updated"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	}
	export class GptPoolResp {
	    id: number;
	    email: string;
	    status: string;
	    plan_type?: string;
	    chatgpt_account_id?: string;
	    success_count: number;
	    failure_count: number;
	    notes?: string;
	    has_access_token: boolean;
	    has_refresh_token: boolean;
	    registered_at: number;
	    expires_at?: number;
	    last_refresh_at?: number;
	    image_quota_remaining?: number;
	    image_quota_total?: number;
	    image_quota_reset_at?: number;
	    last_quota_check_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new GptPoolResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.status = source["status"];
	        this.plan_type = source["plan_type"];
	        this.chatgpt_account_id = source["chatgpt_account_id"];
	        this.success_count = source["success_count"];
	        this.failure_count = source["failure_count"];
	        this.notes = source["notes"];
	        this.has_access_token = source["has_access_token"];
	        this.has_refresh_token = source["has_refresh_token"];
	        this.registered_at = source["registered_at"];
	        this.expires_at = source["expires_at"];
	        this.last_refresh_at = source["last_refresh_at"];
	        this.image_quota_remaining = source["image_quota_remaining"];
	        this.image_quota_total = source["image_quota_total"];
	        this.image_quota_reset_at = source["image_quota_reset_at"];
	        this.last_quota_check_at = source["last_quota_check_at"];
	    }
	}
	export class GptPoolStatsResp {
	    total: number;
	    valid: number;
	    invalid: number;
	    disabled: number;
	    cooldown: number;
	
	    static createFrom(source: any = {}) {
	        return new GptPoolStatsResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.valid = source["valid"];
	        this.invalid = source["invalid"];
	        this.disabled = source["disabled"];
	        this.cooldown = source["cooldown"];
	    }
	}
	export class GptQuotaResp {
	    ok: boolean;
	    plan_type?: string;
	    image_quota_remaining?: number;
	    image_quota_total?: number;
	    image_quota_reset_at?: number;
	    weekly_remaining?: number;
	    weekly_reset_at?: number;
	    credits_balance?: string;
	    default_model?: string;
	    checked_at?: number;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new GptQuotaResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.plan_type = source["plan_type"];
	        this.image_quota_remaining = source["image_quota_remaining"];
	        this.image_quota_total = source["image_quota_total"];
	        this.image_quota_reset_at = source["image_quota_reset_at"];
	        this.weekly_remaining = source["weekly_remaining"];
	        this.weekly_reset_at = source["weekly_reset_at"];
	        this.credits_balance = source["credits_balance"];
	        this.default_model = source["default_model"];
	        this.checked_at = source["checked_at"];
	        this.message = source["message"];
	    }
	}
	export class GptRefreshResp {
	    ok: boolean;
	    expires_at?: number;
	    refreshed_at?: number;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new GptRefreshResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.expires_at = source["expires_at"];
	        this.refreshed_at = source["refreshed_at"];
	        this.message = source["message"];
	    }
	}
	export class MailPoolImportResult {
	    imported: number;
	    skipped: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new MailPoolImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	}
	export class MailPoolResp {
	    id: number;
	    email: string;
	    client_id: string;
	    mode: string;
	    status: string;
	    failure_count: number;
	    last_error?: string;
	    used_by_provider?: string;
	    used_by_account_id?: number;
	    imported_at: number;
	    used_at?: number;
	    registered_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new MailPoolResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.client_id = source["client_id"];
	        this.mode = source["mode"];
	        this.status = source["status"];
	        this.failure_count = source["failure_count"];
	        this.last_error = source["last_error"];
	        this.used_by_provider = source["used_by_provider"];
	        this.used_by_account_id = source["used_by_account_id"];
	        this.imported_at = source["imported_at"];
	        this.used_at = source["used_at"];
	        this.registered_at = source["registered_at"];
	    }
	}
	export class MailPoolStatsResp {
	    total: number;
	    available: number;
	    in_use: number;
	    registered: number;
	    failed: number;
	    disabled: number;
	
	    static createFrom(source: any = {}) {
	        return new MailPoolStatsResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.available = source["available"];
	        this.in_use = source["in_use"];
	        this.registered = source["registered"];
	        this.failed = source["failed"];
	        this.disabled = source["disabled"];
	    }
	}
	export class RegisterTaskCreateResp {
	    created: number;
	    ids: number[];
	
	    static createFrom(source: any = {}) {
	        return new RegisterTaskCreateResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.created = source["created"];
	        this.ids = source["ids"];
	    }
	}
	export class RegisterTaskLogResp {
	    id: number;
	    task_id: number;
	    provider?: string;
	    level: string;
	    step?: string;
	    progress?: number;
	    message?: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RegisterTaskLogResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.provider = source["provider"];
	        this.level = source["level"];
	        this.step = source["step"];
	        this.progress = source["progress"];
	        this.message = source["message"];
	        this.created_at = source["created_at"];
	    }
	}
	export class RegisterTaskResp {
	    id: number;
	    provider: string;
	    status: string;
	    step?: string;
	    progress: number;
	    mail_id?: number;
	    email?: string;
	    payload?: Record<string, any>;
	    result?: Record<string, any>;
	    error?: string;
	    pool_account_id?: number;
	    cancel_requested: boolean;
	    created_at: number;
	    started_at?: number;
	    finished_at?: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RegisterTaskResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.status = source["status"];
	        this.step = source["step"];
	        this.progress = source["progress"];
	        this.mail_id = source["mail_id"];
	        this.email = source["email"];
	        this.payload = source["payload"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.pool_account_id = source["pool_account_id"];
	        this.cancel_requested = source["cancel_requested"];
	        this.created_at = source["created_at"];
	        this.started_at = source["started_at"];
	        this.finished_at = source["finished_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class RegisterTaskStatsResp {
	    total: number;
	    pending: number;
	    running: number;
	    success: number;
	    failed: number;
	    cancelled: number;
	
	    static createFrom(source: any = {}) {
	        return new RegisterTaskStatsResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.pending = source["pending"];
	        this.running = source["running"];
	        this.success = source["success"];
	        this.failed = source["failed"];
	        this.cancelled = source["cancelled"];
	    }
	}

}

export namespace mailbox {
	
	export class MailMessage {
	    id: string;
	    folder: string;
	    subject: string;
	    from: string;
	    received: string;
	    preview: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new MailMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folder = source["folder"];
	        this.subject = source["subject"];
	        this.from = source["from"];
	        this.received = source["received"];
	        this.preview = source["preview"];
	        this.body = source["body"];
	    }
	}

}

export namespace main {
	
	export class AppConfig {
	    provider: string;
	    baseUrl: string;
	    apiKey: string;
	    defaultModel: string;
	    defaultImageModel: string;
	    defaultQuality: string;
	    defaultSize: string;
	    defaultVideoModel: string;
	    defaultDuration: number;
	    defaultRatio: string;
	    defaultVideoQuality: string;
	    asyncImages: boolean;
	    asyncVideos: boolean;
	    callbackUrl: string;
	    outputDir: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.defaultModel = source["defaultModel"];
	        this.defaultImageModel = source["defaultImageModel"];
	        this.defaultQuality = source["defaultQuality"];
	        this.defaultSize = source["defaultSize"];
	        this.defaultVideoModel = source["defaultVideoModel"];
	        this.defaultDuration = source["defaultDuration"];
	        this.defaultRatio = source["defaultRatio"];
	        this.defaultVideoQuality = source["defaultVideoQuality"];
	        this.asyncImages = source["asyncImages"];
	        this.asyncVideos = source["asyncVideos"];
	        this.callbackUrl = source["callbackUrl"];
	        this.outputDir = source["outputDir"];
	    }
	}
	export class GenerationRequest {
	    mode: string;
	    prompt: string;
	    model: string;
	    quality: string;
	    size: string;
	    count: number;
	    duration: number;
	    ratio: string;
	    imagePath?: string;
	    imagePaths?: string[];
	    maskPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.prompt = source["prompt"];
	        this.model = source["model"];
	        this.quality = source["quality"];
	        this.size = source["size"];
	        this.count = source["count"];
	        this.duration = source["duration"];
	        this.ratio = source["ratio"];
	        this.imagePath = source["imagePath"];
	        this.imagePaths = source["imagePaths"];
	        this.maskPath = source["maskPath"];
	    }
	}
	export class GenerationResponse {
	    jobId: string;
	    status: string;
	    progress: number;
	    retryAfter: number;
	    imageUrls: string[];
	    videoUrls: string[];
	    coverUrls: string[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.retryAfter = source["retryAfter"];
	        this.imageUrls = source["imageUrls"];
	        this.videoUrls = source["videoUrls"];
	        this.coverUrls = source["coverUrls"];
	        this.message = source["message"];
	    }
	}
	export class GptPoolPage {
	    items: dto.GptPoolResp[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new GptPoolPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], dto.GptPoolResp);
	        this.total = source["total"];
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
	export class HistoryItem {
	    id: string;
	    title: string;
	    mode: string;
	    prompt: string;
	    model: string;
	    quality: string;
	    size: string;
	    imageUrl: string;
	    videoUrl: string;
	    coverUrl: string;
	    localPath: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.mode = source["mode"];
	        this.prompt = source["prompt"];
	        this.model = source["model"];
	        this.quality = source["quality"];
	        this.size = source["size"];
	        this.imageUrl = source["imageUrl"];
	        this.videoUrl = source["videoUrl"];
	        this.coverUrl = source["coverUrl"];
	        this.localPath = source["localPath"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ImageModel {
	    id: string;
	    name: string;
	    price1k: string;
	    price2k: string;
	    price4k: string;
	    description: string;
	    sizes: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new ImageModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.price1k = source["price1k"];
	        this.price2k = source["price2k"];
	        this.price4k = source["price4k"];
	        this.description = source["description"];
	        this.sizes = source["sizes"];
	    }
	}
	export class MailPoolPage {
	    items: dto.MailPoolResp[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new MailPoolPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], dto.MailPoolResp);
	        this.total = source["total"];
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
	export class PoolFileResult {
	    account: string;
	    kind: string;
	    conversationId: string;
	    primaryPath: string;
	    primaryName: string;
	    zipPath: string;
	    zipName: string;
	
	    static createFrom(source: any = {}) {
	        return new PoolFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.kind = source["kind"];
	        this.conversationId = source["conversationId"];
	        this.primaryPath = source["primaryPath"];
	        this.primaryName = source["primaryName"];
	        this.zipPath = source["zipPath"];
	        this.zipName = source["zipName"];
	    }
	}
	export class PoolGenImage {
	    dataUrl: string;
	    localPath: string;
	    width: number;
	    height: number;
	    mime: string;
	
	    static createFrom(source: any = {}) {
	        return new PoolGenImage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataUrl = source["dataUrl"];
	        this.localPath = source["localPath"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.mime = source["mime"];
	    }
	}
	export class PoolGenModel {
	    id: string;
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new PoolGenModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class PoolGenResult {
	    account: string;
	    model: string;
	    images: PoolGenImage[];
	
	    static createFrom(source: any = {}) {
	        return new PoolGenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.model = source["model"];
	        this.images = this.convertValues(source["images"], PoolGenImage);
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
	export class RegisterTaskPage {
	    items: dto.RegisterTaskResp[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new RegisterTaskPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], dto.RegisterTaskResp);
	        this.total = source["total"];
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
	export class VideoModel {
	    id: string;
	    name: string;
	    description: string;
	    pricing: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.pricing = source["pricing"];
	    }
	}

}

