import assert from 'node:assert/strict';
import test from 'node:test';
import {setButtonPending} from '../src/button-state.ts';

test('pending buttons expose loading state to assistive technology and styling', () => {
    const button = createButton();

    setButtonPending(button, true);

    assert.equal(button.attributes.get('aria-busy'), 'true');
    assert.equal(button.classes.has('is-pending'), true);
});

test('settled buttons clear their loading state', () => {
    const button = createButton();
    setButtonPending(button, true);

    setButtonPending(button, false);

    assert.equal(button.attributes.has('aria-busy'), false);
    assert.equal(button.classes.has('is-pending'), false);
});

const createButton = () => {
    const classes = new Set();
    const attributes = new Map();

    return {
        attributes,
        classes,
        classList: {
            add: (className) => classes.add(className),
            remove: (className) => classes.delete(className),
            toggle: (className, enabled) => enabled ? classes.add(className) : classes.delete(className),
        },
        setAttribute: (name, value) => attributes.set(name, value),
        removeAttribute: (name) => attributes.delete(name),
    };
};
