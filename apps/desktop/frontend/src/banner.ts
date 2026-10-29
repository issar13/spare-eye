import type {app} from '../wailsjs/go/models';

export type Tone = 'plain' | 'good' | 'warn' | 'error';

// showBanner is how every panel reports the outcome of an action. Wails offers
// native dialogs, but a modal for "Saved" would interrupt somebody part way
// through changing a second setting.
export const showBanner = (element: HTMLElement, message: string, tone: Tone = 'plain'): void => {
    element.className = tone === 'plain' ? 'banner' : `banner ${tone}`;
    element.replaceChildren(document.createTextNode(message));
    element.hidden = false;
};

// showResult renders what the Go side decided to tell the user: a headline in
// plain words, the next thing to try, and the guide to read if there is one.
// Nothing else is allowed on screen. An engine error string never reaches here.
export const showResult = (element: HTMLElement, result: app.ActionResult): void => {
    element.className = result.ok ? 'banner good' : 'banner error';
    element.hidden = false;

    const headline = document.createElement('strong');
    headline.textContent = result.headline;

    const parts: Node[] = [headline];

    if (result.nextStep !== '') {
        const nextStep = document.createElement('span');
        nextStep.textContent = result.nextStep;
        parts.push(nextStep);
    }

    if (result.docPath !== '') {
        const guide = document.createElement('span');
        guide.className = 'guide';
        guide.textContent = `Step-by-step instructions are in ${result.docPath}`;
        parts.push(guide);
    }

    element.replaceChildren(...parts);
};

export const hideBanner = (element: HTMLElement): void => {
    element.hidden = true;
};
