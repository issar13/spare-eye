// Typed lookups for the markup in index.html. The panels address elements by id
// dozens of times, and an unchecked querySelector would hand each of them a
// nullable value to re-test.

export const requireElement = <ElementType extends Element>(selector: string): ElementType => {
    const found = document.querySelector<ElementType>(selector);
    if (found === null) {
        throw new Error(`index.html is missing ${selector}`);
    }

    return found;
};

export const requireField = (form: HTMLFormElement, name: string): HTMLInputElement => {
    const field = form.elements.namedItem(name);
    if (!(field instanceof HTMLInputElement)) {
        throw new Error(`form is missing an input named ${name}`);
    }

    return field;
};
