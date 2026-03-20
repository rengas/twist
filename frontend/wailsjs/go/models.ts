export namespace pkg {
	
	export class ChatMessage {
	    id: number;
	    task_id: number;
	    role: string;
	    content: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.created_at = source["created_at"];
	    }
	}
	export class DBStatus {
	    connected: boolean;
	    database_url: string;
	
	    static createFrom(source: any = {}) {
	        return new DBStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.database_url = source["database_url"];
	    }
	}
	export class DesignVersion {
	    id: number;
	    version: number;
	    content: string;
	    task_id: number;
	    created_at: string;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new DesignVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.content = source["content"];
	        this.task_id = source["task_id"];
	        this.created_at = source["created_at"];
	        this.summary = source["summary"];
	    }
	}
	export class Task {
	    id: number;
	    title: string;
	    prompt: string;
	    spec: string;
	    branch: string;
	    pr_url: string;
	    status: string;
	    approved: boolean;
	    session_id: string;
	    chat_session_id: string;
	    worktree_path: string;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.prompt = source["prompt"];
	        this.spec = source["spec"];
	        this.branch = source["branch"];
	        this.pr_url = source["pr_url"];
	        this.status = source["status"];
	        this.approved = source["approved"];
	        this.session_id = source["session_id"];
	        this.chat_session_id = source["chat_session_id"];
	        this.worktree_path = source["worktree_path"];
	    }
	}

}

