// A panel, exported as the module default.
export default class Panel {
  open = false;

  toggle(): void {
    this.open = !this.open;
  }
}
