export namespace database {
	
	export class Arsip {
	    id: string;
	    nama: string;
	    kategori: string;
	    status: string;
	    file_size: number;
	    created: string;
	    updated: string;
	
	    static createFrom(source: any = {}) {
	        return new Arsip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nama = source["nama"];
	        this.kategori = source["kategori"];
	        this.status = source["status"];
	        this.file_size = source["file_size"];
	        this.created = source["created"];
	        this.updated = source["updated"];
	    }
	}

}

export namespace main {
	
	export class AppStatus {
	    app: string;
	    version: string;
	    os: string;
	    arch: string;
	    started: string;
	    uptime: string;
	    window: string;
	    db_ok: boolean;
	    db_err: string;
	    db_engine: string;
	    db_address: string;
	    db_name: string;
	    db_tls: string;
	    sync_ok: boolean;
	    sync_err: string;
	    sync_on: boolean;
	    last_sync: string;
	    node_id: string;
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = source["app"];
	        this.version = source["version"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.started = source["started"];
	        this.uptime = source["uptime"];
	        this.window = source["window"];
	        this.db_ok = source["db_ok"];
	        this.db_err = source["db_err"];
	        this.db_engine = source["db_engine"];
	        this.db_address = source["db_address"];
	        this.db_name = source["db_name"];
	        this.db_tls = source["db_tls"];
	        this.sync_ok = source["sync_ok"];
	        this.sync_err = source["sync_err"];
	        this.sync_on = source["sync_on"];
	        this.last_sync = source["last_sync"];
	        this.node_id = source["node_id"];
	    }
	}
	export class BackupInfo {
	    name: string;
	    path: string;
	    size: number;
	    created: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.created = source["created"];
	    }
	}
	export class DashboardPoint {
	    day: string;
	    arsip: number;
	    berkas: number;
	
	    static createFrom(source: any = {}) {
	        return new DashboardPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.arsip = source["arsip"];
	        this.berkas = source["berkas"];
	    }
	}
	export class DashboardOverview {
	    total_arsip: number;
	    total_pemberkasan: number;
	    total_users: number;
	    sync_queue_pending: number;
	    last_sync: string;
	    series: DashboardPoint[];
	
	    static createFrom(source: any = {}) {
	        return new DashboardOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_arsip = source["total_arsip"];
	        this.total_pemberkasan = source["total_pemberkasan"];
	        this.total_users = source["total_users"];
	        this.sync_queue_pending = source["sync_queue_pending"];
	        this.last_sync = source["last_sync"];
	        this.series = this.convertValues(source["series"], DashboardPoint);
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

