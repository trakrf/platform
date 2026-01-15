import { Component, ErrorInfo, ReactNode } from 'react';
import * as Sentry from '@sentry/react';

interface Props {
  children: ReactNode;
  name?: string;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error(`Error in ${this.props.name || 'component'}:`, error, errorInfo);

    // Report to Sentry with component context
    Sentry.withScope((scope) => {
      scope.setTag('component', this.props.name || 'unknown');
      scope.setExtra('componentStack', errorInfo.componentStack);
      Sentry.captureException(error);
    });
  }

  public render() {
    if (this.state.hasError) {
      return (
        <div style={{ 
          padding: '10px', 
          backgroundColor: '#ffcccc', 
          border: '2px solid red',
          margin: '10px'
        }}>
          <h3>Error in {this.props.name || 'component'}</h3>
          <pre style={{ color: 'red' }}>{this.state.error?.toString()}</pre>
          <details>
            <summary>Stack trace</summary>
            <pre style={{ fontSize: '10px' }}>{this.state.error?.stack}</pre>
          </details>
        </div>
      );
    }

    return this.props.children;
  }
}