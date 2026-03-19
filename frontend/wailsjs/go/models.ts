export namespace pkg {
	
	export class Task {
	    id: number;
	    title: string;
	    prompt: string;
	    spec: string;
	    branch: string;
	    pr_url: string;
	    status: string;
	    approved: boolean;
	
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
	    }
	}

}

