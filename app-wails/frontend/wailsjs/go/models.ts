export namespace main {
	
	export class AppSettings {
	    host: string;
	    port: string;
	    forwardPorts: string;
	    user: string;
	    ipVersion: string;
	    forwardMode: string;
	    keyPath: string;
	    verbose: boolean;
	    useWslSsh: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.forwardPorts = source["forwardPorts"];
	        this.user = source["user"];
	        this.ipVersion = source["ipVersion"];
	        this.forwardMode = source["forwardMode"];
	        this.keyPath = source["keyPath"];
	        this.verbose = source["verbose"];
	        this.useWslSsh = source["useWslSsh"];
	    }
	}
	export class P2PTURNServer {
	    url: string;
	    username: string;
	    credential: string;
	
	    static createFrom(source: any = {}) {
	        return new P2PTURNServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.username = source["username"];
	        this.credential = source["credential"];
	    }
	}
	export class P2PSessionConfig {
	    turnServers: P2PTURNServer[];
	    tcpMuxPort: number;
	    gatherTimeoutSecs: number;
	
	    static createFrom(source: any = {}) {
	        return new P2PSessionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turnServers = this.convertValues(source["turnServers"], P2PTURNServer);
	        this.tcpMuxPort = source["tcpMuxPort"];
	        this.gatherTimeoutSecs = source["gatherTimeoutSecs"];
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
	
	export class RelayCredentials {
	    turnAddr: string;
	    turnTcpAddr: string;
	    username: string;
	    password: string;
	    isRunning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RelayCredentials(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turnAddr    = source["turnAddr"];
	        this.turnTcpAddr = source["turnTcpAddr"] ?? '';
	        this.username    = source["username"];
	        this.password    = source["password"];
	        this.isRunning   = source["isRunning"];
	    }
	}
	export class TunnelSettings {
	    host: string;
	    port: string;
	    forwardPorts: string;
	    user: string;
	    ipVersion: string;
	    forwardMode: string;
	    keyPath: string;
	    verbose: boolean;
	    useWslSsh: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TunnelSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.forwardPorts = source["forwardPorts"];
	        this.user = source["user"];
	        this.ipVersion = source["ipVersion"];
	        this.forwardMode = source["forwardMode"];
	        this.keyPath = source["keyPath"];
	        this.verbose = source["verbose"];
	        this.useWslSsh = source["useWslSsh"];
	    }
	}

}

