// CSS modules import as a map of local name to generated class name. Declared here so feed-core
// type-checks on its own, without depending on a consumer's bundler types.
declare module '*.module.css' {
  const classes: { readonly [key: string]: string };
  export default classes;
}
