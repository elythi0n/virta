import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';
import styles from './Markdown.module.css';

// Custom renderers that map markdown elements to design-system–styled elements.
const components: Components = {
  // Headings — tightly spaced, no giant h1
  h1: ({ children }) => <h1 className={styles.h1}>{children}</h1>,
  h2: ({ children }) => <h2 className={styles.h2}>{children}</h2>,
  h3: ({ children }) => <h3 className={styles.h3}>{children}</h3>,

  // Paragraphs
  p: ({ children }) => <p className={styles.p}>{children}</p>,

  // Bold / italic
  strong: ({ children }) => <strong className={styles.strong}>{children}</strong>,
  em: ({ children }) => <em className={styles.em}>{children}</em>,

  // Inline code and fenced code blocks
  code: ({ className, children, ...rest }) => {
    // react-markdown marks fenced blocks with a language-* className
    const isBlock = 'node' in rest;
    const lang = className?.replace('language-', '') ?? '';
    if (isBlock) {
      return (
        <div className={styles.codeBlock}>
          {lang && <span className={styles.codeLang}>{lang}</span>}
          <code className={styles.codeContent}>{children}</code>
        </div>
      );
    }
    return <code className={styles.inlineCode}>{children}</code>;
  },
  pre: ({ children }) => <pre className={styles.pre}>{children}</pre>,

  // Lists
  ul: ({ children }) => <ul className={styles.ul}>{children}</ul>,
  ol: ({ children }) => <ol className={styles.ol}>{children}</ol>,
  li: ({ children }) => <li className={styles.li}>{children}</li>,

  // Blockquote
  blockquote: ({ children }) => <blockquote className={styles.blockquote}>{children}</blockquote>,

  // Table (GFM)
  table: ({ children }) => <div className={styles.tableWrap}><table className={styles.table}>{children}</table></div>,
  thead: ({ children }) => <thead>{children}</thead>,
  tbody: ({ children }) => <tbody>{children}</tbody>,
  tr:   ({ children }) => <tr className={styles.tr}>{children}</tr>,
  th:   ({ children }) => <th className={styles.th}>{children}</th>,
  td:   ({ children }) => <td className={styles.td}>{children}</td>,

  // Links — open external in new tab
  a: ({ href, children }) => (
    <a className={styles.link} href={href ?? '#'} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),

  // Horizontal rule
  hr: () => <hr className={styles.hr} />,
};

export default function Markdown({ children }: { children: string }) {
  return (
    <div className={styles.root}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
