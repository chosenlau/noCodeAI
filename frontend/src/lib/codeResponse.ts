interface HtmlCodeResponse {
  htmlCode?: unknown;
}

export function extractHtmlCode(response: string): string {
  const trimmed = response.trim();
  const json = trimmed
    .replace(/^```(?:json)?\s*/i, '')
    .replace(/\s*```$/, '')
    .trim();

  try {
    const parsed = JSON.parse(json) as HtmlCodeResponse;
    return typeof parsed.htmlCode === 'string' ? parsed.htmlCode : response;
  } catch {
    return response;
  }
}
