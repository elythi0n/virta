import type { IDockviewHeaderActionsProps } from 'dockview';
import { Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { useTheme } from '../theme';
import styles from './HeaderActions.module.css';

// Right-side header control for every group. Pops the group out into its own window (which maps
// to a native window on desktop). The button only shows while the group is docked in the grid;
// once it is floating or popped out, dragging its tab back into the grid re-docks it (native).
export default function HeaderActions(props: IDockviewHeaderActionsProps) {
  const { theme } = useTheme();

  if (props.location && props.location.type !== 'grid') return null;

  const popOut = () => {
    void props.containerApi.addPopoutGroup(props.group, {
      // The popout window starts from a blank document; carry the current theme across.
      onDidOpen: ({ window }) => {
        window.document.documentElement.dataset.theme = theme;
      },
    });
  };

  return (
    <Tooltip content="Pop out to window" side="bottom">
      <button className={styles.action} aria-label="Pop out to window" onClick={popOut}>
        <Icon name="popout" size={15} />
      </button>
    </Tooltip>
  );
}
