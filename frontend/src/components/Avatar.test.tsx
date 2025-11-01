import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { Avatar } from './Avatar';

afterEach(() => {
  cleanup();
});

describe('Avatar', () => {
  it('should display correct initials for standard email (first.last@domain)', () => {
    render(<Avatar email="john.doe@example.com" />);
    expect(screen.getByText('JD')).toBeInTheDocument();
  });

  it('should display correct initials for underscore separator', () => {
    render(<Avatar email="jane_smith@example.com" />);
    expect(screen.getByText('JS')).toBeInTheDocument();
  });

  it('should display correct initials for hyphen separator', () => {
    render(<Avatar email="bob-jones@example.com" />);
    expect(screen.getByText('BJ')).toBeInTheDocument();
  });

  it('should fallback to first 2 chars if no separators', () => {
    render(<Avatar email="alice@example.com" />);
    expect(screen.getByText('AL')).toBeInTheDocument();
  });

  it('should handle single letter email', () => {
    render(<Avatar email="a@example.com" />);
    expect(screen.getByText('A')).toBeInTheDocument();
  });

  it('should handle email with multiple parts (take first 2)', () => {
    render(<Avatar email="john.paul.smith@example.com" />);
    expect(screen.getByText('JP')).toBeInTheDocument();
  });

  it('should apply custom className', () => {
    const { container } = render(<Avatar email="test@example.com" className="custom-class" />);
    const avatar = container.querySelector('.custom-class');
    expect(avatar).toBeInTheDocument();
  });
});
