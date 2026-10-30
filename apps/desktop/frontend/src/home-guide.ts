import type {app} from '../wailsjs/go/models';

export const guideText = (problem: app.Problem | null | undefined): string => {
    const path = problem?.docPath ?? '';
    return path === '' ? '' : `Step-by-step instructions are in ${path}`;
};
