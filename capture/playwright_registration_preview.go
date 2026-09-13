package capture

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

// This fixed reader never reads input.value, checked, selected, form data,
// attributes outside the public definition allowlist, or arbitrary page text.
const registrationControlDefinition = `element => {
 const tag = element.tagName.toLowerCase(), type = (element.getAttribute('type') || '').toLowerCase();
 let kind = 'unsupported';
 if (tag === 'textarea') kind = 'text';
 if (tag === 'input') {
  if (['text','email','password','number','checkbox'].includes(type)) kind = type;
  else if (!type || ['tel','url','search'].includes(type)) kind = 'text';
  else if (type === 'button') kind = 'button';
 }
 if (tag === 'select' && !element.multiple) kind = 'select';
 if (tag === 'button' && type === 'button') kind = 'button';
 if (tag === 'a' && element.hasAttribute('href') && !element.hasAttribute('download') && (!element.target || element.target === '_self')) kind = 'link';
 const result = {kind};
 if (['text','email','password','number','checkbox','select'].includes(kind)) result.required = element.hasAttribute('required') || element.getAttribute('aria-required') === 'true';
 for (const [attribute, key] of [['minlength','minLength'],['maxlength','maxLength']]) {
  if (['text','email','password'].includes(kind) && element.hasAttribute(attribute)) {
   const raw = element.getAttribute(attribute), number = Number(raw);
   if (raw.trim() === '' || !Number.isSafeInteger(number) || number < 0 || number > 4096) return {kind:'unsupported'};
   result[key] = number;
  }
 }
 if (kind === 'number') for (const [attribute,key] of [['min','minimum'],['max','maximum']]) {
  if (element.hasAttribute(attribute)) {
   const raw = element.getAttribute(attribute), number = Number(raw);
   if (raw.trim() === '' || !Number.isFinite(number) || Math.abs(number)>9007199254740991) return {kind:'unsupported'};
   result[key] = number;
  }
 }
 if (kind === 'select') result.options = Array.from(element.options).filter(option => !option.disabled && option.value !== '').slice(0,33).map(option => ({value:option.value,label:option.label.trim()}));
 return result;
}`

func registrationControlMetadata(locator playwright.Locator) (*registrationauthorsession.ControlMetadata, error) {
	value, err := locator.Evaluate(registrationControlDefinition, nil)
	if err != nil {
		return nil, errors.New("public registration control unavailable")
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > 32<<10 {
		return nil, errors.New("public registration control exceeds bounds")
	}
	var control registrationauthorsession.ControlMetadata
	if json.Unmarshal(data, &control) != nil {
		return nil, errors.New("public registration control invalid")
	}
	if registrationauthorsession.ValidateControlMetadata(&control) != nil {
		return &registrationauthorsession.ControlMetadata{Kind: "unsupported"}, nil
	}
	return &control, nil
}

func (s *playwrightRegistrationSession) Preview(ctx context.Context, candidate registrationauthorsession.Candidate, request registrationauthorsession.PreviewRequest) error {
	if (normalizedRegistrationProtocol(s.request.Protocol) != registrationauthorsession.ProtocolV3 && s.request.Protocol != registrationauthorsession.ProtocolV4) || registrationauthorsession.ValidatePreview(candidate, request) != nil {
		return errors.New("registration preview unsupported")
	}
	if err := s.health(ctx); err != nil {
		return err
	}
	locator := s.page.GetByRole(playwright.AriaRole(candidate.Role), playwright.PageGetByRoleOptions{Name: candidate.Label, Exact: playwright.Bool(true)})
	count, err := locator.Count()
	if err != nil || count != 1 {
		return errors.New("registration preview target ambiguous")
	}
	visible, err := locator.IsVisible()
	if err != nil || !visible {
		return errors.New("registration preview target unavailable")
	}
	control, err := registrationControlMetadata(locator)
	if err != nil || !reflect.DeepEqual(control, candidate.Control) {
		return errors.New("registration preview definition changed")
	}
	timeout, err := operationTimeout(ctx, s.request.NavigationTimeout)
	if err != nil {
		return err
	}
	// All preview actions may cause a reviewed read-only transition. The same
	// exact-origin/GET-HEAD guard remains active throughout the settled action.
	if err := s.guard.beginNavigation(s.page.URL()); err != nil {
		return err
	}
	defer s.guard.endNavigation()
	switch request.Action {
	case "select":
		_, err = locator.SelectOption(playwright.SelectOptionValues{Values: &[]string{*request.Option}}, playwright.LocatorSelectOptionOptions{Timeout: playwright.Float(timeout)})
	case "check":
		err = locator.SetChecked(*request.Checked, playwright.LocatorSetCheckedOptions{Timeout: playwright.Float(timeout)})
	case "click":
		err = locator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(timeout)})
	default:
		return errors.New("registration preview action unsupported")
	}
	if err != nil {
		return errors.New("registration preview failed")
	}
	if err := s.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateLoad, Timeout: playwright.Float(timeout)}); err != nil {
		return errors.New("registration preview transition failed")
	}
	return s.health(ctx)
}
