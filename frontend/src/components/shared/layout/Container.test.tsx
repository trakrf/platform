import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { Container } from './Container';

describe('Container', () => {
  afterEach(() => {
    cleanup();
  });
  it('renders children correctly', () => {
    render(
      <Container>
        <div>Test content</div>
      </Container>
    );

    expect(screen.getByText('Test content')).toBeInTheDocument();
  });

  it('applies variant classes correctly', () => {
    const { rerender, container } = render(
      <Container variant="white">
        <div>Content</div>
      </Container>
    );

    let containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-white');

    rerender(
      <Container variant="gray">
        <div>Content</div>
      </Container>
    );

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-gray-50');

    rerender(
      <Container variant="transparent">
        <div>Content</div>
      </Container>
    );

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('bg-transparent');
  });

  it('applies padding classes correctly', () => {
    const { rerender, container } = render(
      <Container padding="none">
        <div>Content</div>
      </Container>
    );

    let containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).not.toContain('p-3');
    expect(containerDiv.className).not.toContain('p-4');
    expect(containerDiv.className).not.toContain('p-6');

    rerender(
      <Container padding="small">
        <div>Content</div>
      </Container>
    );

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('p-3');

    rerender(
      <Container padding="large">
        <div>Content</div>
      </Container>
    );

    containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('p-6');
  });

  it('applies border when border prop is true', () => {
    const { container } = render(
      <Container border={true}>
        <div>Content</div>
      </Container>
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('border');
    expect(containerDiv.className).toContain('border-gray-200');
  });

  it('removes border when border prop is false', () => {
    const { container } = render(
      <Container border={false}>
        <div>Content</div>
      </Container>
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).not.toContain('border');
  });

  it('applies rounded when rounded prop is true', () => {
    const { container } = render(
      <Container rounded={true}>
        <div>Content</div>
      </Container>
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('rounded-lg');
  });

  it('removes rounded when rounded prop is false', () => {
    const { container } = render(
      <Container rounded={false}>
        <div>Content</div>
      </Container>
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).not.toContain('rounded-lg');
  });

  it('applies custom className', () => {
    const { container } = render(
      <Container className="custom-class">
        <div>Content</div>
      </Container>
    );

    const containerDiv = container.firstChild as HTMLElement;
    expect(containerDiv.className).toContain('custom-class');
  });
});
