// Browsers request /favicon.ico directly (for example when viewing an image or a PDF). The icon is
// generated from the business name at /icon, so point them there.
export function GET(request: Request) {
  return Response.redirect(new URL("/icon", request.url), 308);
}
