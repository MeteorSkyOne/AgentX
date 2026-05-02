declare module "react-syntax-highlighter/dist/esm/prism-light" {
  export { PrismLight as default } from "react-syntax-highlighter";
}

declare module "react-syntax-highlighter/dist/esm/styles/prism/one-dark" {
  const style: Record<string, React.CSSProperties>;
  export default style;
}

declare module "react-syntax-highlighter/dist/esm/styles/prism/one-light" {
  const style: Record<string, React.CSSProperties>;
  export default style;
}

declare module "react-syntax-highlighter/dist/esm/languages/prism/*" {
  const language: unknown;
  export default language;
}
