export namespace app {

	export class ActionResult {
	    ok: boolean;
	    headline: string;
	    nextStep: string;
	    docPath: string;

	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.headline = source["headline"];
	        this.nextStep = source["nextStep"];
	        this.docPath = source["docPath"];
	    }
	}
	export class EventView {
	    kind: string;
	    // Go type: time
	    at: any;
	    message: string;
	    mediaName: string;
	    mediaPath: string;
	    night: boolean;

	    static createFrom(source: any = {}) {
	        return new EventView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.at = this.convertValues(source["at"], null);
	        this.message = source["message"];
	        this.mediaName = source["mediaName"];
	        this.mediaPath = source["mediaPath"];
	        this.night = source["night"];
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
	export class LiveViewForm {
	    previewFPS: number;
	    previewQuality: number;

	    static createFrom(source: any = {}) {
	        return new LiveViewForm(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.previewFPS = source["previewFPS"];
	        this.previewQuality = source["previewQuality"];
	    }
	}
	export class Problem {
	    headline: string;
	    nextStep: string;
	    docPath: string;

	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.headline = source["headline"];
	        this.nextStep = source["nextStep"];
	        this.docPath = source["docPath"];
	    }
	}
	export class RecordingView {
	    name: string;
	    path: string;
	    url: string;
	    sizeMegabytes: number;
	    createdAt: string;
	    manual: boolean;
	    video: boolean;

	    static createFrom(source: any = {}) {
	        return new RecordingView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.url = source["url"];
	        this.sizeMegabytes = source["sizeMegabytes"];
	        this.createdAt = source["createdAt"];
	        this.manual = source["manual"];
	        this.video = source["video"];
	    }
	}
	export class SensitivityForm {
	    minimumContourArea: number;
	    thresholdValue: number;
	    sampleIntervalSeconds: number;
	    alertCooldownSeconds: number;

	    static createFrom(source: any = {}) {
	        return new SensitivityForm(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minimumContourArea = source["minimumContourArea"];
	        this.thresholdValue = source["thresholdValue"];
	        this.sampleIntervalSeconds = source["sampleIntervalSeconds"];
	        this.alertCooldownSeconds = source["alertCooldownSeconds"];
	    }
	}
	export class StartupForm {
	    launchAtLogin: boolean;
	    watchOnLaunch: boolean;

	    static createFrom(source: any = {}) {
	        return new StartupForm(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.launchAtLogin = source["launchAtLogin"];
	        this.watchOnLaunch = source["watchOnLaunch"];
	    }
	}
	export class TuningForm {
	    day: SensitivityForm;
	    night: SensitivityForm;
	    nightStartMinutes: number;
	    nightEndMinutes: number;
	    motionPersistSeconds: number;
	    prebufferLeadSeconds: number;
	    clipDurationSeconds: number;
	    retentionDays: number;
	    maxStorageGigabytes: number;

	    static createFrom(source: any = {}) {
	        return new TuningForm(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = this.convertValues(source["day"], SensitivityForm);
	        this.night = this.convertValues(source["night"], SensitivityForm);
	        this.nightStartMinutes = source["nightStartMinutes"];
	        this.nightEndMinutes = source["nightEndMinutes"];
	        this.motionPersistSeconds = source["motionPersistSeconds"];
	        this.prebufferLeadSeconds = source["prebufferLeadSeconds"];
	        this.clipDurationSeconds = source["clipDurationSeconds"];
	        this.retentionDays = source["retentionDays"];
	        this.maxStorageGigabytes = source["maxStorageGigabytes"];
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
	export class TelegramForm {
	    botToken: string;
	    chatID: string;
	    enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new TelegramForm(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.botToken = source["botToken"];
	        this.chatID = source["chatID"];
	        this.enabled = source["enabled"];
	    }
	}
	export class SettingsView {
	    telegram: TelegramForm;
	    tuning: TuningForm;
	    liveView: LiveViewForm;
	    startup: StartupForm;
	    cameraIndex: number;
	    cameraURL: string;
	    cameraChosen: boolean;
	    mediaDirectory: string;

	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.telegram = this.convertValues(source["telegram"], TelegramForm);
	        this.tuning = this.convertValues(source["tuning"], TuningForm);
	        this.liveView = this.convertValues(source["liveView"], LiveViewForm);
	        this.startup = this.convertValues(source["startup"], StartupForm);
	        this.cameraIndex = source["cameraIndex"];
	        this.cameraURL = source["cameraURL"];
	        this.cameraChosen = source["cameraChosen"];
	        this.mediaDirectory = source["mediaDirectory"];
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

	export class StatusView {
	    headline: string;
	    tone: string;
	    explanation: string;
	    problem?: Problem;
	    armed: boolean;
	    recording: boolean;
	    recordingManual: boolean;
	    night: boolean;
	    motionSeconds: number;
	    captureFPS: number;
	    frameWidth: number;
	    frameHeight: number;
	    cameraSource: string;
	    alertsEnabled: boolean;
	    alertsConfigured: boolean;
	    telegramConnected: boolean;
	    launchAtLogin: boolean;
	    launchAtLoginRegistered: boolean;
	    canArm: boolean;
	    lastFailure: string;
	    logPath: string;
	    mediaDirectory: string;

	    static createFrom(source: any = {}) {
	        return new StatusView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.headline = source["headline"];
	        this.tone = source["tone"];
	        this.explanation = source["explanation"];
	        this.problem = this.convertValues(source["problem"], Problem);
	        this.armed = source["armed"];
	        this.recording = source["recording"];
	        this.recordingManual = source["recordingManual"];
	        this.night = source["night"];
	        this.motionSeconds = source["motionSeconds"];
	        this.captureFPS = source["captureFPS"];
	        this.frameWidth = source["frameWidth"];
	        this.frameHeight = source["frameHeight"];
	        this.cameraSource = source["cameraSource"];
	        this.alertsEnabled = source["alertsEnabled"];
	        this.alertsConfigured = source["alertsConfigured"];
	        this.telegramConnected = source["telegramConnected"];
	        this.launchAtLogin = source["launchAtLogin"];
	        this.launchAtLoginRegistered = source["launchAtLoginRegistered"];
	        this.canArm = source["canArm"];
	        this.lastFailure = source["lastFailure"];
	        this.logPath = source["logPath"];
	        this.mediaDirectory = source["mediaDirectory"];
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

export namespace detector {

	export class Camera {
	    index: number;
	    width: number;
	    height: number;
	    label: string;

	    static createFrom(source: any = {}) {
	        return new Camera(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.label = source["label"];
	    }
	}

}

export namespace onvif {

	export class Camera {
	    name: string;
	    address: string;
	    serviceURL: string;

	    static createFrom(source: any = {}) {
	        return new Camera(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.address = source["address"];
	        this.serviceURL = source["serviceURL"];
	    }
	}

}
