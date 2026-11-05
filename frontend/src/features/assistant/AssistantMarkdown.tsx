import { memo } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

/** No raw HTML, images, automatic remote resources, or clickable model routes.
 * Links are rendered as text: desktop navigation keeps its existing grant gate. */
export const AssistantMarkdown = memo(function AssistantMarkdown({ text }: { text: string }) {
  return <div className="min-w-0 break-words text-sm leading-7 [overflow-wrap:anywhere] [&_p]:my-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-1 [&_h1]:my-3 [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:my-3 [&_h2]:font-semibold [&_h3]:font-semibold [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_pre]:overflow-x-auto [&_pre]:rounded [&_pre]:bg-muted [&_pre]:p-3 [&_code]:font-mono [&_code]:text-xs [&_table]:block [&_table]:overflow-x-auto [&_th]:border [&_th]:border-border [&_th]:px-2 [&_td]:border [&_td]:border-border [&_td]:px-2">
    <Markdown skipHtml remarkPlugins={[remarkGfm]} urlTransform={() => ""}
      components={{ a: ({ children }) => <span>{children}</span>, img: ({ alt }) => <span>{alt}</span> }}
      allowedElements={["p", "strong", "em", "del", "blockquote", "code", "pre", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "br", "table", "thead", "tbody", "tr", "th", "td", "a", "img", "sup", "section"]}>
      {text}
    </Markdown>
  </div>;
});
