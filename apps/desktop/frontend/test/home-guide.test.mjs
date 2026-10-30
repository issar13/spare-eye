import assert from 'node:assert/strict';
import test from 'node:test';
import {guideText} from '../src/home-guide.ts';

test('a missing Wails problem hides the help guide', () => {
    assert.equal(guideText(null), '');
});

test('a problem with a help document links to that document', () => {
    assert.equal(
        guideText({docPath: 'docs/camera-setup.md'}),
        'Step-by-step instructions are in docs/camera-setup.md',
    );
});
