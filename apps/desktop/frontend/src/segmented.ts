import {requireElement} from './elements';

// A segmented control is a row of named choices standing in for numbers nobody
// wants to reason about. The sensitivity profiles and the live view both need
// one and differ only in their levels, so the choices are passed in.

type Choice = {label: string; description: string};

export const renderSegmented = <Level extends string>(
    selector: string,
    order: Level[],
    choices: Record<Level, Choice>,
    selected: Level,
    onChoose: (level: Level) => void,
): void => {
    const container = requireElement<HTMLElement>(selector);

    container.replaceChildren(...order.map((level) => {
        const preset = choices[level];

        const choice = document.createElement('button');
        choice.type = 'button';
        choice.className = level === selected ? 'choice selected' : 'choice';
        choice.setAttribute('aria-pressed', String(level === selected));
        choice.addEventListener('click', () => onChoose(level));

        const label = document.createElement('strong');
        label.textContent = preset.label;

        const description = document.createElement('span');
        description.textContent = preset.description;

        choice.append(label, description);
        return choice;
    }));
};
