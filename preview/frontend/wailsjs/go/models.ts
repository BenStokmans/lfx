export namespace main {
	
	export class LayoutData {
	    id: string;
	    name: string;
	    builtIn: boolean;
	    path?: string;
	    width: number;
	    height: number;
	    points: runtime.Point[];
	
	    static createFrom(source: any = {}) {
	        return new LayoutData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.builtIn = source["builtIn"];
	        this.path = source["path"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.points = this.convertValues(source["points"], runtime.Point);
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
	export class EffectFileData {
	    name: string;
	    path: string;
	    relativePath: string;
	    modulePath?: string;
	
	    static createFrom(source: any = {}) {
	        return new EffectFileData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.relativePath = source["relativePath"];
	        this.modulePath = source["modulePath"];
	    }
	}
	export class WorkspaceData {
	    root: string;
	    effects: EffectFileData[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.effects = this.convertValues(source["effects"], EffectFileData);
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
	export class BootstrapData {
	    defaultWorkspace: string;
	    workspace?: WorkspaceData;
	    layouts: LayoutData[];
	
	    static createFrom(source: any = {}) {
	        return new BootstrapData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultWorkspace = source["defaultWorkspace"];
	        this.workspace = this.convertValues(source["workspace"], WorkspaceData);
	        this.layouts = this.convertValues(source["layouts"], LayoutData);
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
	export class CompileRequest {
	    workspaceRoot: string;
	    filePath: string;
	    overrides: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new CompileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspaceRoot = source["workspaceRoot"];
	        this.filePath = source["filePath"];
	        this.overrides = source["overrides"];
	    }
	}
	export class DiagnosticData {
	    severity: string;
	    code?: string;
	    message: string;
	    filePath?: string;
	    line?: number;
	    column?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.code = source["code"];
	        this.message = source["message"];
	        this.filePath = source["filePath"];
	        this.line = source["line"];
	        this.column = source["column"];
	    }
	}
	export class PresetData {
	    name: string;
	    speed: number;
	    start: number;
	    loopStart: number;
	    loopEnd: number;
	    finish: number;
	
	    static createFrom(source: any = {}) {
	        return new PresetData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.speed = source["speed"];
	        this.start = source["start"];
	        this.loopStart = source["loopStart"];
	        this.loopEnd = source["loopEnd"];
	        this.finish = source["finish"];
	    }
	}
	export class ParamData {
	    name: string;
	    type: string;
	    defaultValue: any;
	    min?: number;
	    max?: number;
	    enumValues?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ParamData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.defaultValue = source["defaultValue"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.enumValues = source["enumValues"];
	    }
	}
	export class CompileResponse {
	    compilationId?: string;
	    workspaceRoot: string;
	    filePath: string;
	    modulePath?: string;
	    wgsl?: string;
	    params: ParamData[];
	    boundParams?: Record<string, any>;
	    presets: PresetData[];
	    diagnostics: DiagnosticData[];
	
	    static createFrom(source: any = {}) {
	        return new CompileResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.compilationId = source["compilationId"];
	        this.workspaceRoot = source["workspaceRoot"];
	        this.filePath = source["filePath"];
	        this.modulePath = source["modulePath"];
	        this.wgsl = source["wgsl"];
	        this.params = this.convertValues(source["params"], ParamData);
	        this.boundParams = source["boundParams"];
	        this.presets = this.convertValues(source["presets"], PresetData);
	        this.diagnostics = this.convertValues(source["diagnostics"], DiagnosticData);
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
	
	
	
	
	
	export class SamplePointData {
	    index: number;
	    x: number;
	    y: number;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new SamplePointData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.value = source["value"];
	    }
	}
	export class SampleRequest {
	    compilationId: string;
	    layout: LayoutData;
	    phase: number;
	    overrides: Record<string, any>;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new SampleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.compilationId = source["compilationId"];
	        this.layout = this.convertValues(source["layout"], LayoutData);
	        this.phase = source["phase"];
	        this.overrides = source["overrides"];
	        this.limit = source["limit"];
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
	export class SampleResponse {
	    points: SamplePointData[];
	
	    static createFrom(source: any = {}) {
	        return new SampleResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.points = this.convertValues(source["points"], SamplePointData);
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
	export class SaveSourceRequest {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveSourceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}

}

export namespace runtime {
	
	export class Point {
	    Index: number;
	    X: number;
	    Y: number;
	
	    static createFrom(source: any = {}) {
	        return new Point(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Index = source["Index"];
	        this.X = source["X"];
	        this.Y = source["Y"];
	    }
	}

}

