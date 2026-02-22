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

