import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

/*
 * Ответы модели приходят в Markdown (заголовки, списки, **жирный**, таблицы).
 * react-markdown не исполняет сырой HTML из текста, поэтому ответ модели безопасно
 * рендерить как есть. Недописанный во время стрима Markdown отрисовывается как обычный текст.
 */
const components: Components = {
  a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noopener noreferrer" />,
  // таблица прокручивается внутри пузыря, а не растягивает ленту
  table: ({ node: _node, ...props }) => (
    <div className="md-table">
      <table {...props} />
    </div>
  ),
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
}
