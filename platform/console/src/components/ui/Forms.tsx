'use client';

import React, { useId } from 'react';

type FieldProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'id'> & {
  label: string;
  hint?: string;
  error?: string;
};

export function Field({ label, hint, error, className = '', ...props }: FieldProps) {
  const id = useId();
  const description = error || hint;
  return (
    <div className={`k-field ${className}`.trim()}>
      <label className="k-field-label" htmlFor={id}>{label}</label>
      <input
        {...props}
        id={id}
        className="k-field-input"
        aria-invalid={error ? true : undefined}
        aria-describedby={description ? `${id}-description` : undefined}
      />
      {description && (
        <span id={`${id}-description`} className={error ? 'k-field-error' : 'k-field-hint'}>
          {description}
        </span>
      )}
    </div>
  );
}

type TextAreaProps = Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'id'> & {
  label: string;
  hint?: string;
  error?: string;
};

export function TextArea({ label, hint, error, className = '', ...props }: TextAreaProps) {
  const id = useId();
  const description = error || hint;
  return (
    <div className={`k-field ${className}`.trim()}>
      <label className="k-field-label" htmlFor={id}>{label}</label>
      <textarea
        {...props}
        id={id}
        className="k-field-input k-field-textarea"
        aria-invalid={error ? true : undefined}
        aria-describedby={description ? `${id}-description` : undefined}
      />
      {description && (
        <span id={`${id}-description`} className={error ? 'k-field-error' : 'k-field-hint'}>
          {description}
        </span>
      )}
    </div>
  );
}

export function SearchField(props: Omit<FieldProps, 'type'>) {
  return <Field {...props} type="search" />;
}

type SelectProps = Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'id'> & {
  label: string;
  hint?: string;
  error?: string;
  options: { value: string; label: string; disabled?: boolean }[];
};

export function Select({ label, hint, error, options, className = '', ...props }: SelectProps) {
  const id = useId();
  const description = error || hint;
  return (
    <div className={`k-field ${className}`.trim()}>
      <label className="k-field-label" htmlFor={id}>{label}</label>
      <select
        {...props}
        id={id}
        className="k-field-input k-field-select"
        aria-invalid={error ? true : undefined}
        aria-describedby={description ? `${id}-description` : undefined}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value} disabled={option.disabled}>{option.label}</option>
        ))}
      </select>
      {description && (
        <span id={`${id}-description`} className={error ? 'k-field-error' : 'k-field-hint'}>
          {description}
        </span>
      )}
    </div>
  );
}

export function Checkbox({
  label,
  className = '',
  ...props
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> & { label: string }) {
  return (
    <label className={`k-choice ${className}`.trim()}>
      <input {...props} type="checkbox" className="k-choice-input" />
      <span>{label}</span>
    </label>
  );
}

export function Toggle({
  label,
  className = '',
  ...props
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'role'> & { label: string }) {
  return (
    <label className={`k-choice k-toggle ${className}`.trim()}>
      <input {...props} type="checkbox" role="switch" className="k-choice-input" />
      <span className="k-toggle-track" aria-hidden="true" />
      <span>{label}</span>
    </label>
  );
}

export function Slider({
  label,
  className = '',
  ...props
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> & { label: string }) {
  const id = useId();
  return (
    <div className={`k-field ${className}`.trim()}>
      <label className="k-field-label" htmlFor={id}>{label}</label>
      <input {...props} id={id} type="range" className="k-slider" />
    </div>
  );
}
